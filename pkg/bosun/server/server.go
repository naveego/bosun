package server

import (
	"fmt"
	"github.com/naveego/bosun/pkg/core"
	"net"
	"net/http"
	"strings"
	"sync"
)

type Server struct {
	url string
	server *http.Server
	valueFiles map[string][]byte
}

var defaultServer *Server
var defaultServerInit = new(sync.Once)

func GetDefaultServer() *Server {
	defaultServerInit.Do(func(){
		defaultServer = newServer()
	})

	return defaultServer
}

func newServer() *Server {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		panic(err)
	}

	httpServer := &http.Server{	}

	core.Log.Infof("Value file server started at %s", listener.Addr().String())

	go func(){
		err = httpServer.Serve(listener)
		if err != nil {
			panic(err)
		}
	}()

	s := &Server{
		server: httpServer,
		valueFiles: map[string][]byte{},
		url:fmt.Sprintf("http://%s", listener.Addr().String()),
	}
	mux := http.NewServeMux()
	mux.Handle("values/", http.HandlerFunc(s.HandleValueFileRequest))

	return s
}

// Adds a value file and returns the URL to it.
func (s *Server) AddValueFile(name string, b []byte) string {
	s.valueFiles[name] = b
	return fmt.Sprintf("%s/%s", s.url, name)
}

func (s *Server) HandleValueFileRequest(resp http.ResponseWriter, req *http.Request) {

	segs := strings.Split(req.URL.Path, "/")
	if len(segs) != 2 {
		_, _ = fmt.Fprintf(resp, "Invalid value path (should be values/filename, was %q).", req.URL.Path)
		resp.WriteHeader(400)
		return
	}

	valueFileName := segs[1]

	if valueFileBytes, ok := s.valueFiles[valueFileName]; ok {
		resp.Write(valueFileBytes)
		resp.WriteHeader(200)
		return
	}

	_, _ = fmt.Fprintf(resp, "Value file %q not registered.", valueFileName)
	resp.WriteHeader(404)
}