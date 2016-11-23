package gorsyncclient

import (
	"reflect"
	"testing"
	"time"
)

func TestRsyncClient_parameterChecker(t *testing.T) {
	type fields struct {
		srcDirPath   string
		tcpAddr      string
		speedLimit   int
		postWorkers  int
		scanInterval time.Duration
	}
	tests := []struct {
		name    string
		fields  fields
		wantErr bool
	}{
		// TODO: Add test cases.
		{"bibinbin", fields{"./", "127.0.0.1:7878", 2, 0, time.Second}, false},
		{"bibinbin", fields{"./", "", 2, 0, time.Second}, true},
		{"bibinbin", fields{"./", "", -1, 0, time.Second}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rc := &RsyncClient{
				srcDirPath:   tt.fields.srcDirPath,
				tcpAddr:      tt.fields.tcpAddr,
				speedLimit:   tt.fields.speedLimit,
				postWorkers:  tt.fields.postWorkers,
				scanInterval: tt.fields.scanInterval,
			}
			if err := rc.parameterChecker(); (err != nil) != tt.wantErr {
				t.Errorf("RsyncClient.parameterChecker() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestRsyncClient_getFileList(t *testing.T) {
	type fields struct {
		srcDirPath   string
		tcpAddr      string
		speedLimit   int
		postWorkers  int
		scanInterval time.Duration
		// quit              chan struct{}
		filesMap map[string]*MapFileInfo
		// pool              tcppool.TcpPool
		filesPathChannels chan string
		// done              sync.WaitGroup
	}
	type args struct {
		dirPath string
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    []string
		wantErr bool
	}{
		// TODO: Add test cases.
		{"bibinbin", fields{"./", "127.0.0.1:7878", 2, 0, time.Second, make(map[string]*MapFileInfo, 0), make(chan string, 0)}, args{"./"}, []string{}, false},
		{"bibinbin", fields{"", "127.0.0.1:7878", 2, 0, time.Second, make(map[string]*MapFileInfo, 0), make(chan string, 0)}, args{""}, []string{}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rc := &RsyncClient{
				srcDirPath:   tt.fields.srcDirPath,
				tcpAddr:      tt.fields.tcpAddr,
				speedLimit:   tt.fields.speedLimit,
				postWorkers:  tt.fields.postWorkers,
				scanInterval: tt.fields.scanInterval,
				// quit:              tt.fields.quit,
				filesMap: tt.fields.filesMap,
				// pool:              tt.fields.pool,
				filesPathChannels: tt.fields.filesPathChannels,
				// done:              tt.fields.done,
			}
			got, err := rc.getFileList(tt.args.dirPath)
			if (err != nil) != tt.wantErr {
				t.Errorf("RsyncClient.getFileList() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, got) {
				t.Errorf("RsyncClient.getFileList() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRecurseListDir(t *testing.T) {
	type args struct {
		dirPath string
	}
	tests := []struct {
		name    string
		args    args
		want    []string
		wantErr bool
	}{
		// TODO: Add test cases.
		{"bibinbin", args{"./"}, []string{}, false},
		{"bibinbin", args{}, []string{}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := RecurseListDir(tt.args.dirPath)
			if (err != nil) != tt.wantErr {
				t.Errorf("RecurseListDir() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, got) {
				t.Errorf("RecurseListDir() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestExist(t *testing.T) {
	type args struct {
		filename string
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{"bibinbin", args{"test.log"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Exist(tt.args.filename); got != tt.want {
				t.Errorf("Exist() = %v, want %v", got, tt.want)
			}
		})
	}
}
