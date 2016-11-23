package gorsyncclient

import (
	"compress/gzip"
	"encoding/gob"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/bibinbin/tcppool"
	"github.com/golang/glog"
	"github.com/juju/ratelimit" //令牌桶限速
)

// 必须实现的方法
type GoRsyncClient interface {
	StartRsyncClient() (err error)                                                                                      // start
	StopRsyncClient()                                                                                                   // stop
	RestartRsyncClient(srcDirPath, tcpaddr string, speedLimit, postWorkers int, scanInterval time.Duration) (err error) // restart
}

type RsyncClient struct {
	srcDirPath        string                  // 本地文件夹
	tcpAddr           string                  // tcpaddr
	speedLimit        int                     // 上传限速
	postWorkers       int                     // 发送文件的线程数
	scanInterval      time.Duration           // 扫描目标目录的频率
	quit              chan struct{}           // 服务退出信号
	filesMap          map[string]*MapFileInfo // 用于存储文件信息
	pool              tcppool.TcpPool         // 连接池
	filesPathChannels chan string
	done              sync.WaitGroup
}

// initial rsyncclient
func NewRsyncClient(srcDirPath, tcpaddr string, speedLimit, postWorkers int, scanInterval time.Duration) (GoRsyncClient, error) {
	rc := new(RsyncClient)
	rc.tcpAddr = tcpaddr
	rc.speedLimit = speedLimit
	rc.postWorkers = postWorkers
	rc.scanInterval = scanInterval
	rc.srcDirPath, _ = filepath.Abs(srcDirPath)
	if err := rc.parameterChecker(); err != nil {
		return rc, err
	}
	return rc, nil
}

// 开启客户端
func (rc *RsyncClient) StartRsyncClient() (err error) {
	factory := func() (net.Conn, error) { return net.Dial("tcp", rc.tcpAddr) }
	rc.pool, err = tcppool.NewChannelPool(0, rc.postWorkers, factory) //初始化，tcppool
	if err != nil {
		return err
	}
	rc.quit = make(chan struct{})                              //初始化退出标志
	rc.filesMap = make(map[string]*MapFileInfo)                //全局文件
	rc.filesPathChannels = make(chan string, 2*rc.postWorkers) //初始化带缓冲的chanel
	rc.done.Add(1 + rc.postWorkers)
	go rc.ScanDirWorker()
	for i := 0; i < rc.postWorkers; i++ {
		go rc.SendWorker()
	}
	rc.done.Wait()
	return nil
}

func (rc *RsyncClient) RestartRsyncClient(srcDirPath, tcpaddr string, speedLimit, postWorkers int, scanInterval time.Duration) (err error) {
	rc.StopRsyncClient()
	rc.tcpAddr = tcpaddr
	rc.speedLimit = speedLimit
	rc.postWorkers = postWorkers
	rc.scanInterval = scanInterval
	rc.srcDirPath, _ = filepath.Abs(srcDirPath)
	if err := rc.parameterChecker(); err != nil {
		return err
	}
	return rc.StartRsyncClient()
}

// 关闭客户端
func (rc *RsyncClient) StopRsyncClient() {
	close(rc.quit)              // 发出退出信号
	close(rc.filesPathChannels) // 关闭缓冲channel
	rc.done.Wait()              // 等待程序执行完毕
	rc.pool.Close()             // 关闭连接池
}

// 目录扫描
func (rc *RsyncClient) ScanDirWorker() {
	defer rc.done.Done()
	for {
		select {
		case <-rc.quit:
			return
		default:
			if files, err := rc.getFileList(rc.srcDirPath); err != nil {
				glog.Errorln(err)
				time.Sleep(rc.scanInterval) // 出错时休息一会
			} else if len(files) == 0 {
				time.Sleep(rc.scanInterval) // 目录为空时休息一会
			} else {
				for _, file := range files {
					select {
					case <-rc.quit:
						return
					default:
						rc.filesPathChannels <- file
					}
				}
			}
		}
	}
}

// 文件发送器
func (rc *RsyncClient) SendWorker() {
	fmt.Println(`send worker...`)
	defer rc.done.Done()
	receiveBuf := make([]byte, ReceiveBufferSize)
	readBuf := make([]byte, ReadBufferSize)
	dp := DataPackage{}
	var encoder *gob.Encoder
	var gz *gzip.Writer
	for filePath := range rc.filesPathChannels {
		// 打开文件
		fp, err := os.Open(filePath)
		if err != nil {
			delete(rc.filesMap, filePath)
			continue
		}
		// 从连接池中获取一个连接对象
		conn, err := rc.pool.Get()
		if err != nil {
			glog.Errorln(err)
			fp.Close()
			rc.filesMap[filePath].Sending = false
			time.Sleep(time.Second * RetryTime)
			continue
		}

		// 发送给服务端ready信号
		if _, err := conn.Write([]byte("Ready")); err != nil {
			glog.Errorln(err)
			fp.Close()
			conn.Close()
			rc.filesMap[filePath].Sending = false
			continue
		}

		// 接收服务端返回的信号
		if count, err := conn.Read(receiveBuf); err != nil && err != io.EOF {
			glog.Errorln(err)
			fp.Close()
			conn.Close()
			rc.filesMap[filePath].Sending = false
			continue
		} else if string(receiveBuf[:count]) != "Go" {
			glog.Errorln(string(receiveBuf[:count]))
			fp.Close()
			conn.Close()
			rc.filesMap[filePath].Sending = false
			continue
		}

		// 设置最大一天的超时时间;发送文件信息
		conn.SetWriteDeadline(time.Now().Add(24 * time.Hour))
		sendFileInfo, fileInfo, err := rc.getFileInfo(fp, filePath)
		if err != nil {
			glog.Errorln(err)
			fp.Close()
			delete(rc.filesMap, filePath)
			conn.Close()
			continue
		}
		if _, err := conn.Write(sendFileInfo); err != nil {
			glog.Errorln(err)
			fp.Close()
			rc.filesMap[filePath].Sending = false
			conn.Close()
			continue
		}

		// 读取服务端传递过来的信号
		if count, err := conn.Read(receiveBuf); err != nil && err != io.EOF {
			glog.Errorln(err)
			fp.Close()
			conn.Close()
			rc.filesMap[filePath].Sending = false
			continue
		} else if string(receiveBuf[:count]) != "ComeOn" {
			glog.Errorln(receiveBuf[:count])
			fp.Close()
			conn.Close()
			rc.filesMap[filePath].Sending = false
			if string(receiveBuf[:count]) == `Error: file already exists` {
				delete(rc.filesMap, filePath)
			}
			continue
		}

		// 发送文件数据
		if err := rc.sendFileContent(gz, encoder, conn, fileInfo, fp, dp, readBuf); err != nil {
			glog.Errorln(err)
			rc.filesMap[filePath].Sending = false
			fp.Close()
			conn.Close()
			continue
		}
		fp.Close()
		count, err := conn.Read(receiveBuf)
		if (err != nil && err != io.EOF) || string(receiveBuf[:count]) != "Complete" {
			glog.Errorln(err, string(receiveBuf[:count]))
			rc.filesMap[filePath].Sending = false
			conn.Close()
			continue
		}
		os.Remove(filePath)
		delete(rc.filesMap, filePath)
		// 链接回归池子
		rc.pool.Put(conn)
	}
}

// get file info
func (rc *RsyncClient) getFileInfo(file *os.File, filePath string) ([]byte, os.FileInfo, error) {
	fileInfo, err := file.Stat()
	if err != nil {
		return []byte(""), fileInfo, err
	}
	sendFileInfo, err := json.Marshal(
		SendFileInfo{
			FileName: strings.Split(filePath[len(rc.srcDirPath):], string(os.PathSeparator)),
			Size:     fileInfo.Size(),
		})
	return sendFileInfo, fileInfo, err
}

// send limit
func (rc *RsyncClient) sendFileContent(gz *gzip.Writer, encoder *gob.Encoder, conn net.Conn, fileInfo os.FileInfo, fp *os.File, dp DataPackage, readBuf []byte) error {
	if rc.speedLimit != 0 {
		bucket := ratelimit.NewBucketWithRate(float64(rc.speedLimit*1024), int64(rc.speedLimit*1024))
		lw := ratelimit.Writer(conn, bucket)
		gz = gzip.NewWriter(lw)
		encoder = gob.NewEncoder(gz)
	} else {
		gz = gzip.NewWriter(conn)
		encoder = gob.NewEncoder(gz)
	}
	packNum := (fileInfo.Size()-1)/ReadBufferSize + 1
	for i := int64(0); i < packNum; i++ {
		count, err := fp.Read(readBuf)
		if err != nil && err != io.EOF {
			return err
		}
		dp.Data = readBuf[:count]
		err = encoder.Encode(dp)
		if err != nil {
			return err
		}
		gz.Flush()
	}
	return nil
}
