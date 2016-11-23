package gorsyncserver

import (
	"log"
	"os"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/gorsync/gorsyncclient" //引入gorsyncclient包
)

var rs GoRsyncServer
var rc gorsyncclient.GoRsyncClient
var waitting sync.WaitGroup

func init() {
	var err error
	if rs, err = NewRsyncServer(`./receive`, `0.0.0.0:7878`, 120, 256, []string{`127.0.0.1`}, false); err != nil {
		log.Fatal(err)
	}
	if rc, err = gorsyncclient.NewRsyncClient("./send", "127.0.0.1:7878", 120, 0, 2*time.Second); err != nil {
		log.Fatal(err)
	}
}

func TestNewRsyncServer(t *testing.T) {
	type args struct {
		dstDirPath      string
		tcpHost         string
		speedLimit      int
		maxConns        int
		allowedIP       []string
		ipCertification bool
	}
	tests := []struct {
		name    string
		args    args
		want    GoRsyncServer
		wantErr bool
	}{
		{"bibinbin", args{`./`, `0.0.0.0:7878`, 120, 256, []string{`127.0.0.1`}, false}, rs, false},
		{"bibinbin", args{`./`, `0.0.0.0:7878`, 120, 256, []string{}, true}, rs, true},
		{"bibinbin", args{``, ``, 120, 256, []string{}, false}, rs, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NewRsyncServer(tt.args.dstDirPath, tt.args.tcpHost, tt.args.speedLimit, tt.args.maxConns, tt.args.allowedIP, tt.args.ipCertification)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewRsyncServer() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, got) { //有一处开辟的地址单元是不一样的
				t.Errorf("NewRsyncServer() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRsyncServer_parameterChecker(t *testing.T) {
	type fields struct {
		dstDirPath      string
		tcpHost         string
		speedLimit      int
		allowedIp       []string
		ipCertification bool
		//done            sync.WaitGroup
		//quit            chan struct{}
		//listen           *net.TCPListener
		//connLimitChannel chan bool
	}
	tests := []struct {
		name    string
		fields  fields
		wantErr bool
	}{
		// TODO: Add test cases.
		{"bibinbin", fields{`./`, `0.0.0.0:7878`, 256, []string{`127.0.0.1`}, false}, false},
		{"bibinbin", fields{`./`, `0.0.0.0:7878`, 256, []string{}, true}, true},
		{"bibinbin", fields{``, ``, 256, []string{}, false}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rs := &RsyncServer{
				dstDirPath:      tt.fields.dstDirPath,
				tcpHost:         tt.fields.tcpHost,
				speedLimit:      tt.fields.speedLimit,
				allowedIp:       tt.fields.allowedIp,
				ipCertification: tt.fields.ipCertification,
				//done:            tt.fields.done,
				//quit:             tt.fields.quit,
				//listen:           tt.fields.listen,
				//connLimitChannel: tt.fields.connLimitChannel,
			}
			if err := rs.parameterChecker(); (err != nil) != tt.wantErr {
				t.Errorf("RsyncServer.parameterChecker() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestListenAndServer(t *testing.T) {
	type args struct {
		dstDirPath      string
		tcpHost         string
		speedLimit      int
		maxConns        int
		allowedIP       []string
		ipCertification bool
	}
	tests := []struct {
		name    string
		args    args
		want    GoRsyncServer
		wantErr bool
	}{
		{"bibinbin", args{`./`, `7878`, 120, 256, []string{`127.0.0.1`}, false}, rs, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NewRsyncServer(tt.args.dstDirPath, tt.args.tcpHost, tt.args.speedLimit, tt.args.maxConns, tt.args.allowedIP, tt.args.ipCertification)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewRsyncServer() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, got) { //有一处开辟的地址单元是不一样的
				t.Errorf("NewRsyncServer() = %v, want %v", got, tt.want)
			}
			if err := got.ListenAndServer(); (err != nil) == tt.wantErr {
				t.Error(`i need err`)
			}
		})
	}
}

func TestListenAndServer2(t *testing.T) {
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
		if err := rs.RestartServer(`./receive`, `0.0.0.0:7878`, 120, 256, []string{`127.0.0.1`}, true); err != nil {
			t.Error(err)
		}
	}()
	time.Sleep(2 * time.Minute)
	log.Println(`server restart success`)
	rc.StopRsyncClient()
	rs.StopServer()
	os.RemoveAll(`./send`)
	os.RemoveAll(`./receive`)
	log.Println(`清理现场完毕`)
}

func TestListenAndServer3(t *testing.T) {
	rs, _ := NewRsyncServer(`./receive`, `0.0.0.0:7878`, 0, 256, []string{`10.16.93.246`}, true)
	rc, _ := gorsyncclient.NewRsyncClient("./send", "127.0.0.1:7878", 120, 0, 2*time.Second)
	//log.Println(`清理现场完毕`)
	go rs.ListenAndServer()
	go rc.StartRsyncClient()
	waitting.Add(2)
	os.MkdirAll(`./send`, os.ModePerm)
	if _, err := os.Create(`./send/send.log`); err != nil {
		t.Error(err)
	}
	time.Sleep(3 * time.Second)
	waitting.Done()
	waitting.Done()
	rc.StopRsyncClient()
	rs.StopServer()
	os.RemoveAll(`./send`)
	os.RemoveAll(`./receive`)
	log.Println(`清理现场完毕`)
}
