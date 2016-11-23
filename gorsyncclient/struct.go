package gorsyncclient

// 文件信息
type MapFileInfo struct {
	Sending     bool
	ModUnixTime int64
}

type ValSorter struct {
	Keys []string
	Vals []*MapFileInfo
}

type DataPackage struct {
	Data []byte
}

type SendFileInfo struct {
	FileName []string
	Size     int64
}
