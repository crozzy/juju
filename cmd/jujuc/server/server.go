// The cmd/jujuc/server package allows a process to expose an RPC interface that
// allows client processes to delegate execution of cmd.Commands to a server
// process (with the exposed commands amenable to specialisation by context id).
package server

import (
	"bytes"
	"fmt"
	"launchpad.net/juju/go/cmd"
	"net"
	"net/rpc"
	"os"
	"path/filepath"
	"sync"
)

var jujucPurpose = "invoke a hosted command inside the unit agent process"
var JUJUC_DOC = `
The jujuc command forwards invocations over RPC for execution by another
process. It expects to be called via a symlink named for the desired remote
command, and expects JUJU_AGENT_SOCKET and JUJU_CONTEXT_ID be set in its
environment.
`

// Request contains the information necessary to run a Command remotely.
type Request struct {
	ContextId string
	Dir       string
	Args      []string
}

// Response contains the return code and output generated by a Request.
type Response struct {
	Code   int
	Stdout string
	Stderr string
}

// CmdsGetter returns a list of available cmd.Commands, connected to the
// context identified by contextId.
type CmdsGetter func(contextId string) ([]cmd.Command, error)

// Jujuc wraps a set of Commands for RPC.
type Jujuc struct {
	getCmds CmdsGetter
}

// cmd returns a cmd.Command which can interpret Request arguments and run
// the appropriate subcommand against state specified by contextId.
func (j *Jujuc) cmd(contextId string) (cmd.Command, error) {
	cmds, err := j.getCmds(contextId)
	if err != nil {
		return nil, err
	}
	sc := &cmd.SuperCommand{
		Name: "(-> jujuc)", Purpose: jujucPurpose, Doc: JUJUC_DOC,
	}
	for _, c := range cmds {
		sc.Register(c)
	}
	return sc, nil
}

// badReqErr returns an error indicating a bad Request.
func badReqErr(format string, v ...interface{}) error {
	return fmt.Errorf("bad request: "+format, v...)
}

// Main runs the Command specified by req, and fills in resp.
func (j *Jujuc) Main(req Request, resp *Response) error {
	if req.Args == nil || len(req.Args) < 1 {
		return badReqErr("Args is too short")
	}
	if !filepath.IsAbs(req.Dir) {
		return badReqErr("Dir is not absolute")
	}
	c, err := j.cmd(req.ContextId)
	if err != nil {
		return badReqErr("%s", err)
	}
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	ctx := &cmd.Context{req.Dir, stdout, stderr}
	resp.Code = cmd.Main(c, ctx, req.Args)
	resp.Stdout = stdout.String()
	resp.Stderr = stderr.String()
	return nil
}

// Server wraps net.rpc.Server so as to allow Commands to be executed in one
// process on behalf of another.
type Server struct {
	socketPath string
	listener   net.Listener
	server     *rpc.Server
	closed     chan bool
	closing    chan bool
	wg         sync.WaitGroup
}

// NewServer creates an RPC server bound to socketPath, which can execute
// remote command invocations against an appropriate Context. It will not
// actually do so until Run is called.
func NewServer(getCmds CmdsGetter, socketPath string) (*Server, error) {
	server := rpc.NewServer()
	if err := server.Register(&Jujuc{getCmds}); err != nil {
		return nil, err
	}
	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		return nil, err
	}
	s := &Server{
		socketPath: socketPath,
		listener:   listener,
		server:     server,
		closed:     make(chan bool),
		closing:    make(chan bool),
	}
	return s, nil
}

// Run accepts new connections until it encounters an error, or until Close is
// called, and then blocks until all existing connections have been closed.
func (s *Server) Run() (err error) {
	var conn net.Conn
	for {
		conn, err = s.listener.Accept()
		if err != nil {
			break
		}
		s.wg.Add(1)
		go func(conn net.Conn) {
			s.server.ServeConn(conn)
			s.wg.Done()
		}(conn)
	}
	select {
	case <-s.closing:
		err = nil
	default:
	}
	s.wg.Wait()
	close(s.closed)
	return
}

// Close immediately stops accepting connections, and blocks until all existing
// connections have been closed.
func (s *Server) Close() {
	close(s.closing)
	s.listener.Close()
	os.Remove(s.socketPath)
	<-s.closed
}
