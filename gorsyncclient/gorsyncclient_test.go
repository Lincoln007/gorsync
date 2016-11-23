package gorsyncclient

import (
	"log"
	"os"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/gorsync/gorsyncserver" //引入gorsyncserver包
)

var waitting sync.WaitGroup
var rc GoRsyncClient
var rs gorsyncserver.GoRsyncServer

func init() {
	var err error
	if rc, err = NewRsyncClient("./send", "127.0.0.1:7878", 120, 0, 2*time.Second); err != nil {
		log.Fatal(err)
	}
	if rs, err = gorsyncserver.NewRsyncServer(`./receive`, `0.0.0.0:7878`, 120, 256, []string{`127.0.0.1`}, false); err != nil {
		log.Fatal(err)
	}
}

func TestNewRsyncClient(t *testing.T) {
	type args struct {
		srcDirPath   string
		tcpaddr      string
		speedLimit   int
		postWorkers  int
		scanInterval time.Duration
	}
	tests := []struct {
		name    string
		args    args
		want    GoRsyncClient
		wantErr bool
	}{
		// TODO: Add test cases.
		{"bibinbin", args{"./", "127.0.0.1:7878", 0, 4, time.Second}, rc, false},
		{"bibinbin", args{"./", "", 0, 4, time.Second}, rc, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NewRsyncClient(tt.args.srcDirPath, tt.args.tcpaddr, tt.args.speedLimit, tt.args.postWorkers, tt.args.scanInterval)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewRsyncClient() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, got) {
				t.Errorf("NewRsyncClient() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRsyncClient_StartRsyncClient(t *testing.T) {
	log.Println(`清理现场完毕`)
	go rs.ListenAndServer()
	go rc.StartRsyncClient()
	waitting.Add(2)
	if _, err := os.Create(`./send/send.log`); err != nil {
		log.Fatalln(err)
	}
	if _, err := os.Create(`./send/send2.log`); err != nil {
		log.Fatalln(err)
	}
	if _, err := os.Create(`./receive/send.log`); err != nil {
		log.Fatalln(err)
	}
	time.Sleep(5 * time.Second)
	if _, err := os.Open(`./receive/send.log`); err != nil {
		log.Fatal(err)
	} else {
		log.Println(`文件传输成功`)
	}
	waitting.Done()
	waitting.Done()
	go func() {
		if err := rc.RestartRsyncClient("./send", "127.0.0.1:7878", 120, 0, 2*time.Second); err != nil {
			t.Error(err)
		}
	}()
	time.Sleep(10 * time.Second)
	log.Println(`client restart success`)
	rc.StopRsyncClient()
	rs.StopServer()
	os.RemoveAll(`./send`)
	os.RemoveAll(`./receive`)
	log.Println(`清理现场完毕`)
}
