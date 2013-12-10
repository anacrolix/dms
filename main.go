package main

import (
	"bitbucket.org/anacrolix/dms/dlna/dms"
	"bitbucket.org/anacrolix/dms/rrcache"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"os"
	"os/signal"
	"os/user"
	"path/filepath"
	"runtime"
	"sync"
)

type fFprobeCache struct {
	c *rrcache.RRCache
	sync.Mutex
}

func (fc *fFprobeCache) Get(key interface{}) (value interface{}, ok bool) {
	fc.Lock()
	defer fc.Unlock()
	return fc.c.Get(key)
}

func (fc *fFprobeCache) Set(key interface{}, value interface{}) {
	fc.Lock()
	defer fc.Unlock()
	var size int64
	for _, v := range []interface{}{key, value} {
		b, err := json.Marshal(v)
		if err != nil {
			panic(err)
		}
		size += int64(len(b))
	}
	fc.c.Set(key, value, size)
}

func main() {
	log.SetFlags(log.Ltime | log.Lshortfile)

	path := flag.String("path", "", "browse root path")

	ifName := flag.String("ifname", "", "specific SSDP network interface")
	httpAddr := flag.String("http", ":1338", "http server port")
	friendlyName := flag.String("friendlyName", "", "server friendly name")
	fFprobeCachePath := flag.String("fFprobeCachePath", func() (path string) {
		_user, err := user.Current()
		if err != nil {
			log.Print(err)
			return
		}
		path = filepath.Join(_user.HomeDir, ".dms-ffprobe-cache")
		return
	}(), "path to FFprobe cache file")

	flag.Parse()
	if flag.NArg() != 0 {
		fmt.Fprintf(os.Stderr, "%s: %s\n", "unexpected positional arguments", flag.Args())
		flag.Usage()
		os.Exit(2)
	}

	cache := &fFprobeCache{
		c: rrcache.New(64 << 20),
	}
	if err := loadFFprobeCache(cache, *fFprobeCachePath); err != nil {
		log.Print(err)
	}

	dmsServer := &dms.Server{
		Interfaces: func(ifName string) (ifs []net.Interface) {
			var err error
			if ifName == "" {
				ifs, err = net.Interfaces()
			} else {
				var if_ *net.Interface
				if_, err = net.InterfaceByName(ifName)
				if if_ != nil {
					ifs = append(ifs, *if_)
				}
			}
			if err != nil {
				log.Fatal(err)
			}
			return
		}(*ifName),
		HTTPConn: func() net.Listener {
			conn, err := net.Listen("tcp", *httpAddr)
			if err != nil {
				log.Fatal(err)
			}
			return conn
		}(),
		FriendlyName:   *friendlyName,
		RootObjectPath: filepath.Clean(*path),
		FFProbeCache:   cache,
	}
	go func() {
		if err := dmsServer.Serve(); err != nil {
			log.Fatal(err)
		}
	}()
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, os.Interrupt)
	<-sigs
	err := dmsServer.Close()
	if err != nil {
		log.Fatal(err)
	}
	if err := saveFFprobeCache(cache, *fFprobeCachePath); err != nil {
		log.Print(err)
	}
}

func loadFFprobeCache(cache *fFprobeCache, path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	dec := json.NewDecoder(f)
	var items []dms.FFprobeCacheItem
	err = dec.Decode(&items)
	if err != nil {
		return err
	}
	for _, item := range items {
		cache.Set(item.Key, item.Value)
	}
	log.Printf("added %d items from cache", len(items))
	return nil
}

func saveFFprobeCache(cache *fFprobeCache, path string) error {
	cache.Lock()
	items := cache.c.Items()
	cache.Unlock()
	f, err := ioutil.TempFile(filepath.Dir(path), filepath.Base(path))
	if err != nil {
		return err
	}
	enc := json.NewEncoder(f)
	err = enc.Encode(items)
	f.Close()
	if err != nil {
		os.Remove(f.Name())
		return err
	}
	if runtime.GOOS == "windows" {
		err = os.Remove(path)
		if err == os.ErrNotExist {
			err = nil
		}
	}
	if err == nil {
		err = os.Rename(f.Name(), path)
	}
	if err == nil {
		log.Printf("saved cache with %d items", len(items))
	} else {
		os.Remove(f.Name())
	}
	return err
}
