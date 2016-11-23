package gorsyncserver

type FileStat struct {
	FileName []string
	Size     int64
}

type DataPackage struct {
	Data []byte
}
