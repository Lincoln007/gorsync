package gorsyncserver

import (
	"compress/gzip"
	"encoding/gob"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/golang/glog"
	"github.com/juju/ratelimit" // 令牌桶限速
)

type GoRsyncServer interface {
	ListenAndServer() (err error)
	StopServer()
	RestartServer(dstDirPath, tcpHost string, speedLimit, maxConns int, allowedIP []string, ipCertification bool) error
}

type RsyncServer struct {
	dstDirPath       string           // 文件接收路径
	tcpHost          string           // tcp端口和ip地址
	speedLimit       int              // 限速
	allowedIp        []string         // 允许访问的ip
	ipCertification  bool             // 关闭ip认证 true 开启| false 关闭
	done             sync.WaitGroup   // 携程等待锁🔐
	quit             chan struct{}    // 服务退出信号
	listen           *net.TCPListener // tcp服务
	connLimitChannel chan bool
}

const (
	ReceiveBufSize = 1024 * 1024
)

// 初始化rsyncserver
func NewRsyncServer(dstDirPath, tcpHost string, speedLimit, maxConns int, allowedIP []string, ipCertification bool) (GoRsyncServer, error) {
	flag.Parse()
	rs := new(RsyncServer)
	rs.allowedIp = append(rs.allowedIp, allowedIP...)
	rs.tcpHost = tcpHost
	rs.ipCertification = ipCertification
	rs.speedLimit = speedLimit
	rs.dstDirPath, _ = filepath.Abs(dstDirPath)
	rs.connLimitChannel = make(chan bool, maxConns)
	if err := rs.parameterChecker(); err != nil {
		glog.Warningln(`参数检查失败:` + err.Error())
		return rs, err
	}
	// 修改路径权限
	os.Chmod(rs.dstDirPath, os.ModePerm)
	return rs, nil
}

// 参数检查
func (rs *RsyncServer) parameterChecker() error {
	if rs.tcpHost == `` {
		return errors.New(`Parameters cannot be empty`)
	}
	if rs.ipCertification && len(rs.allowedIp) == 0 {
		return errors.New(`Allows the IP to be empty, or to close the IP authentication`)
	}
	// 创建一个新目录，该目录是利用路径（包括绝对路径和相对路径）进行创建的，如果需要创建对应的父目录，也一起进行创建，如果已经有了该目录，则不进行新的创建，当创建一个已经存在的目录时，不会报错.
	if err := os.MkdirAll(rs.dstDirPath, os.ModePerm); err != nil {
		return err
	}
	return nil
}

// 开启服务
func (rs *RsyncServer) ListenAndServer() (err error) {
	rs.quit = make(chan struct{})
	// 获取一个tcp地址
	addr, err := net.ResolveTCPAddr("tcp", rs.tcpHost)
	if err != nil {
		glog.Warningln(err)
		return err
	}
	// 开启一个tcp服务
	rs.listen, err = net.ListenTCP(`tcp`, addr)
	if err != nil {
		glog.Warningln(`Listener port failed`, err.Error())
		return err
	}
	fmt.Println(`Has initialized connection, waiting for client connection...`)
	glog.Infoln(`Has initialized connection, waiting for client connection...`)
	rs.Server(rs.listen)
	return nil
}

// 重启服务
func (rs *RsyncServer) RestartServer(dstDirPath, tcpHost string, speedLimit, maxConns int, allowedIP []string, ipCertification bool) error {
	rs.StopServer()
	rs.allowedIp = append(rs.allowedIp, allowedIP...)
	rs.tcpHost = tcpHost
	rs.ipCertification = ipCertification
	rs.speedLimit = speedLimit
	rs.dstDirPath, _ = filepath.Abs(dstDirPath)
	rs.connLimitChannel = make(chan bool, maxConns)
	if err := rs.parameterChecker(); err != nil {
		return err
	}
	// 修改路径权限
	os.Chmod(rs.dstDirPath, os.ModePerm)
	return rs.ListenAndServer()

}

// 关闭服务
func (rs *RsyncServer) StopServer() {
	close(rs.quit)
	rs.done.Wait()
	rs.listen.Close()
}

// TCPServer
func (rs *RsyncServer) Server(listen *net.TCPListener) {
	for {
		select {
		case <-rs.quit:
			return
		default:
		}
		// 接受tcp服务请求
		conn, err := listen.AcceptTCP()
		if err != nil {
			glog.Warningln(`Accept client connection exception: `, err.Error())
			continue
		}
		// 各项检查开始
		if !rs.HealthCheck(conn) {
			requestIp := strings.Split(conn.RemoteAddr().String(), ":")[0]
			glog.Infoln(requestIp + ` 访问被拒绝。`)
			continue
		}
		// 各项检查结束
		go rs.Handle(conn)
		rs.done.Add(1)

		select {
		case <-rs.quit:
			return
		default:
		}
	}
}

// true 健康的|false 有病的
func (rs *RsyncServer) HealthCheck(conn net.Conn) bool {
	select {
	case rs.connLimitChannel <- true:
		// 开启ip验证的情况
		if rs.ipCertification {
			requestIp := strings.Split(conn.RemoteAddr().String(), ":")[0]
			for _, ip := range rs.allowedIp {
				if requestIp == ip {
					return true
				}
			}
		}
		// 关闭ip验证的情况
		if !rs.ipCertification {
			return true
		}
	default:
	}
	go rs.refuseHandle(conn)
	rs.done.Add(1)
	return false
}

// 服务端拒绝链接
func (rs *RsyncServer) refuseHandle(conn net.Conn) {
	defer rs.done.Done()
	conn.Write([]byte(`Error: The server exceeds the maximum number of connections Or IP access denied`))
}

// handle
func (rs *RsyncServer) Handle(conn net.Conn) {
	defer rs.done.Done()
	defer conn.Close()
	num := 0
	for {
		select {
		case <-rs.quit:
			return
		default:
			// 设置超时时间
			conn.SetReadDeadline(time.Now().Add(time.Minute))
			receiveBuf := make([]byte, ReceiveBufSize)
			// 等待了一天都没有链接，这个链接设置为过期
			if count, err := conn.Read(receiveBuf); err != nil && err != io.EOF {
				num++
				// 12小时都没东西出来，自动断开，仍然没有结果断开长连接
				if strings.Contains(err.Error(), `timeout`) && num <= 60*12 {
					continue
				}
				return
			} else if string(receiveBuf[:count]) != `Ready` {
				glog.Infoln(`client 主动关闭连接` + err.Error())
				return
			}
			// 通知服务端开始传递文件信息
			if _, err := conn.Write([]byte(`Go`)); err != nil {
				glog.Errorln(err)
				return
			}
			conn.SetDeadline(time.Now().Add(24 * time.Hour))
			count, err := conn.Read(receiveBuf) //读取客户端传递过来的文件信息
			if err != nil && err != io.EOF {
				glog.Errorln(err)
				conn.Write([]byte(`Error: Failed to read file information` + err.Error()))
				return
			}
			// 创建文件
			stat, fp, filePath, tmpPath, err := rs.createFile(receiveBuf[:count])
			if err != nil {
				glog.Errorln(err)
				conn.Write([]byte(err.Error()))
				return
			}
			if _, err := conn.Write([]byte(`ComeOn`)); err != nil {
				glog.Errorln(err)
				return
			}
			// 接收文件
			if err := rs.receiveFileContent(stat, fp, conn); err != nil {
				glog.Errorln(`Error: Failed to read file information` + err.Error())
				fp.Close()
				os.Remove(tmpPath)
				conn.Write([]byte(`Error: Failed to read file information` + err.Error()))
				return
			}
			fp.Close()
			if err := os.Rename(tmpPath, filePath); err != nil {
				glog.Errorln(err)
				os.Remove(tmpPath)
				return
			}
			conn.Write([]byte(`Complete`))
		}
	}
}

// 接收文件头信息
func (rs *RsyncServer) createFile(fileinfo []byte) (stat FileStat, fp *os.File, filePath string, tmpPath string, err error) {
	if err := json.Unmarshal(fileinfo, &stat); err != nil {
		return stat, nil, ``, ``, errors.New(`Error: json deocde err` + err.Error())
	}
	filePath = rs.dstDirPath + strings.Join(stat.FileName, string(os.PathSeparator))
	tmpPath = filePath + `.tmp` //临时tmp文件
	// 文件存在或者有同名文件存在，拒绝传输
	if Exist(filePath) || Exist(tmpPath) {
		tmp := strings.Split(filepath.Base(filePath), `.`)
		tmp[0] = tmp[0] + `(` + time.Now().String() + `)`
		filePath = filepath.Dir(filePath) + string(os.PathSeparator) + strings.Join(tmp, `.`)
		tmpPath = filePath + `.tmp`
		glog.Infoln(filepath.Base(filePath) + `rename to ` + strings.Join(tmp, `.`))
	}
	if err := os.MkdirAll(filepath.Dir(tmpPath), os.ModePerm); err != nil {
		return stat, nil, filePath, tmpPath, errors.New(`Error: mkdir dir err` + err.Error())
	}
	// 创建临时文件
	if fp, err = os.Create(tmpPath); err != nil {
		return stat, nil, filePath, tmpPath, errors.New(`Error: create file err: ` + err.Error())
	}
	return stat, fp, filePath, tmpPath, nil
}

// receive limit
func (rs *RsyncServer) receiveFileContent(stat FileStat, fp *os.File, conn net.Conn) error {
	var decoder *gob.Decoder
	packageBuf := new(DataPackage)

	if rs.speedLimit != 0 {
		bucket := ratelimit.NewBucketWithRate(float64(rs.speedLimit*1024), int64(rs.speedLimit*1024))
		lr := ratelimit.Reader(conn, bucket)
		gz, err := gzip.NewReader(lr)
		if err != nil {
			return err
		}
		decoder = gob.NewDecoder(gz)
	} else {
		gz, err := gzip.NewReader(conn)
		if err != nil {
			return err
		}
		decoder = gob.NewDecoder(gz)
	}
	packNum := (stat.Size-1)/ReceiveBufSize + 1

	for i := int64(0); i < packNum; i++ {
		if err := decoder.Decode(packageBuf); err != nil {
			return err
		}
		fp.Write(packageBuf.Data)
	}
	return nil
}

// 检查文件或目录是否存在
// 如果由 filename 指定的文件或目录存在则返回 true，否则返回 false
func Exist(filename string) bool {
	_, err := os.Stat(filename)
	return err == nil || os.IsExist(err)
}
