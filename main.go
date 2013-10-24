package main

// ---

import (
	"os"
	"io"
	"log"
	"fmt"
	"time"
	"path"
	"regexp"
	"net/http"
	"encoding/hex"
	flags "github.com/jessevdk/go-flags"
	proxy "github.com/websecurify/go-proxy"
)

// ---

type Context interface {
	getTime() int64
}

// ---

type contextData struct {
	time int64
}

func (cd contextData) getTime() int64 {
    return cd.time
}

func ContextData() *contextData {
	cd := new(contextData)
	cd.time = time.Now().UnixNano()
	
	return cd
}

// ---

type teeReadCloser struct {
	r io.ReadCloser
	w io.WriteCloser
}

func (t *teeReadCloser) Read(p []byte) (n int, err error) {
	n, err = t.r.Read(p)
	
	if n > 0 {
		if n, err := t.w.Write(p[:n]); err != nil {
			return n, err
		}
	}
	
	return
}

func (t *teeReadCloser) Close() (err error) {
	err = t.r.Close()
	err = t.w.Close()
	
	return err
}

func TeeReadCloser(r io.ReadCloser, w io.WriteCloser) io.ReadCloser {
	return &teeReadCloser{r, w}
}

// ---

func printRequest(req *http.Request, to io.Writer) {
	to.Write([]byte(fmt.Sprintf("%s %s %s\r\n", req.Method, req.URL, req.Proto)))
	
	for k, v := range req.Header {
		for _, v := range v {
			to.Write([]byte(fmt.Sprintf("%s: %s\r\n", k, v)))
		}
	}
	
	to.Write([]byte("\r\n"))
}

func printResponse(res *http.Response, to io.Writer) {
	to.Write([]byte(fmt.Sprintf("%s %s\n", res.Proto, res.Status)))
	
	for k, v := range res.Header {
		for _, v := range v {
			to.Write([]byte(fmt.Sprintf("%s: %s\n", k, v)))
		}
	}
	
	to.Write([]byte("\n"))
}

// ---

var opts struct {
	Port int `short:"p" long:"port" description:"Choose port to listen to" default:"8080"`
	Dump string `short:"d" long:"dump" description:"Dump requests and responses to files."`
	Mitm bool `short:"m" long:"mitm" description:"Perform ssl man-in-the-middle"`
	Transactions bool `short:"t" long:"transactions" description:"Print transactions"`
	Hexdump bool `short:"x" long:"hexdump" description:"Hex dump both requests and responses"`
	Verbose bool `short:"v" long:"verbose" description:"Show verbose debug information"`
}

// ---

func main() {
	_, err := flags.ParseArgs(&opts, os.Args)
	
	if err != nil {
		os.Exit(1)
	}
	
	p := proxy.NewProxyHttpServer()
	p.Verbose = opts.Verbose
	
	p.OnRequest().DoFunc(func (req *http.Request, ctx *proxy.ProxyCtx) (*http.Request, *http.Response) {
		ctx.UserData = ContextData()
		
		return req, nil
	})
	
	if opts.Mitm {
		p.OnRequest(proxy.ReqHostMatches(regexp.MustCompile("^.*$"))).HandleConnect(proxy.AlwaysMitm)
	}
	
	if opts.Dump != "" {
		stat, err := os.Stat(opts.Dump)
		
		if os.IsNotExist(err) {
			fmt.Fprintf(os.Stderr, "path %s is not found\n", opts.Dump)
			
			os.Exit(1)
		}
		
		if !stat.IsDir() {
			fmt.Fprintf(os.Stderr, "path %s is not directory\n", opts.Dump)
			
			os.Exit(1)
		}
		
		p.OnRequest().DoFunc(func (req *http.Request, ctx *proxy.ProxyCtx) (*http.Request, *http.Response) {
			if req != nil {
				cd := ctx.UserData.(Context)
				file, err := os.Create(path.Join(opts.Dump, fmt.Sprintf("%d.request", cd.getTime())))
				
				if err == nil {
					printRequest(req, file)
					
					if req.Body != nil {
						req.Body = TeeReadCloser(req.Body, file)
					}
				}
			}
			
			return req, nil
		})
		
		p.OnResponse().DoFunc(func (res *http.Response, ctx *proxy.ProxyCtx) *http.Response {
			if res != nil {
				cd := ctx.UserData.(Context)
				file, err := os.Create(path.Join(opts.Dump, fmt.Sprintf("%d.response", cd.getTime())))
				
				if err == nil {
					printResponse(res, file)
					
					if res.Body != nil {
						res.Body = TeeReadCloser(res.Body, file)
					}
				}
			}
			
			return res
		})
	}
	
	if opts.Transactions {
		p.OnRequest().DoFunc(func (req *http.Request, ctx *proxy.ProxyCtx) (*http.Request, *http.Response) {
			printRequest(req, os.Stdout)
			
			return req, nil
		})
		
		p.OnResponse().DoFunc(func (res *http.Response, ctx *proxy.ProxyCtx) *http.Response {
			printResponse(res, os.Stdout)
			
			return res
		})
	}
	
	if opts.Hexdump {
		p.OnRequest().DoFunc(func (req *http.Request, ctx *proxy.ProxyCtx) (*http.Request, *http.Response) {
			if req != nil && req.Body != nil {
				dumper := hex.Dumper(os.Stdout)
				req.Body = TeeReadCloser(req.Body, dumper)
			}
			
			return req, nil
		})
		
		p.OnResponse().DoFunc(func (res *http.Response, ctx *proxy.ProxyCtx) *http.Response {
			if res != nil && res.Body != nil {
				dumper := hex.Dumper(os.Stdout)
				res.Body = TeeReadCloser(res.Body, dumper)
			}
			
			return res
		})
	}
	
	if opts.Verbose {
		log.Print(fmt.Sprintf("listening on %d", opts.Port))
	}
	
	log.Fatal(http.ListenAndServe(fmt.Sprintf(":%d", opts.Port), p))
}
