package gorsyncclient

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
)

const (
	ReadBufferSize       = 1024 * 1024
	ReceiveBufferSize    = 1024 * 1024
	RetryTime            = 30
	FileNameIllegalChars = `/\:*?"<>|`
)

// checker
func (rc *RsyncClient) parameterChecker() error {
	if rc.postWorkers <= 0 {
		rc.postWorkers = runtime.NumCPU()
	}
	if rc.tcpAddr == `` {
		return errors.New(`Tcpaddr cannot be empty`)
	}
	// 创建一个新目录，该目录是利用路径（包括绝对路径和相对路径）进行创建的，如果需要创建对应的父目录，也一起进行创建，如果已经有了该目录，则不进行新的创建，当创建一个已经存在的目录时，不会报错.
	if err := os.MkdirAll(rc.srcDirPath, os.ModePerm); err != nil {
		return err
	}
	if rc.speedLimit < 0 {
		return errors.New(`Upload speed must be greater than 0`)
	}
	return nil
}

// 获取文件夹列表
func (rc *RsyncClient) getFileList(dirPath string) ([]string, error) {
	files, err := RecurseListDir(dirPath)
	if err != nil {
		return nil, err
	}
	for _, file := range files {
		if _, ok := rc.filesMap[file]; ok {
			continue
		}
		_, fileName := filepath.Split(file)
		// 作下过滤，去掉.tmp后缀的文件和包含windows/linux非法文件名字符的文件
		if filepath.Ext(file) != ".tmp" && string(fileName[0]) != "." && !(strings.ContainsAny(fileName, FileNameIllegalChars)) {
			fileinfo, err := os.Stat(file)
			if err != nil && !os.IsExist(err) {
				fmt.Println(err)
				continue
			}
			rc.filesMap[file] = &MapFileInfo{Sending: false, ModUnixTime: fileinfo.ModTime().UnixNano()}
		}
	}
	// 按修改时间排序得到需要发送的文件列表
	ret := make([]string, 0, len(files))
	vs := NewValSorter(rc.filesMap)
	vs.Sort()
	for i, info := range vs.Vals {
		if !info.Sending {
			rc.filesMap[vs.Keys[i]].Sending = true
			ret = append(ret, vs.Keys[i])
		}
	}
	return ret, nil
}

// 递归列出所有文件
func RecurseListDir(dirPath string) ([]string, error) {
	var filePaths []string
	err := filepath.Walk(dirPath,
		func(path string, f os.FileInfo, err error) error {
			if f == nil {
				return err
			}
			if f.IsDir() {
				return nil
			}
			path, _ = filepath.Abs(path)
			filePaths = append(filePaths, path)
			return nil
		})
	return filePaths, err
}

// 判断文件或文件夹是否存在
func Exist(filename string) bool {
	_, err := os.Stat(filename)
	return err == nil || os.IsExist(err)
}

// 对map的值进行排序
func NewValSorter(m map[string]*MapFileInfo) *ValSorter {
	vs := &ValSorter{
		Keys: make([]string, 0, len(m)),
		Vals: make([]*MapFileInfo, 0, len(m)),
	}
	for k, v := range m {
		vs.Keys = append(vs.Keys, k)
		vs.Vals = append(vs.Vals, v)
	}
	return vs
}

func (vs *ValSorter) Sort() {
	sort.Sort(vs)
}

func (vs *ValSorter) Len() int           { return len(vs.Keys) }
func (vs *ValSorter) Less(i, j int) bool { return vs.Vals[i].ModUnixTime < vs.Vals[j].ModUnixTime }
func (vs *ValSorter) Swap(i, j int) {
	vs.Vals[i], vs.Vals[j] = vs.Vals[j], vs.Vals[i]
	vs.Keys[i], vs.Keys[j] = vs.Keys[j], vs.Keys[i]
}
