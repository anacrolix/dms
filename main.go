package main

//go:generate go-bindata data/

import (
	"bytes"
	"encoding/json"
	"flag"
	"io/ioutil"
	"log"
	"net"
	"os"
	"os/signal"
	"os/user"
	"path/filepath"
	"runtime"
	"sync"
	"syscall"

	"github.com/anacrolix/dms/dlna/dms"
	"github.com/anacrolix/dms/rrcache"
)

type dmsConfig struct {
	Path                string
	IfName              string
	Http                string
	FriendlyName        string
	LogHeaders          bool
	FFprobeCachePath    string
	NoTranscode         bool
	StallEventSubscribe bool
}

func (config *dmsConfig) load(configPath string) {
	file, err := os.Open(configPath)
	if err != nil {
		log.Printf("config error (config file: '%s'): %v\n", configPath, err)
		return
	}
	defer file.Close()
	decoder := json.NewDecoder(file)
	err = decoder.Decode(&config)
	if err != nil {
		log.Printf("config error: %v\n", err)
		return
	}
}

//default config
var config = &dmsConfig{
	Path:             "",
	IfName:           "",
	Http:             ":1338",
	FriendlyName:     "",
	LogHeaders:       false,
	FFprobeCachePath: getDefaultFFprobeCachePath(),
}

func getDefaultFFprobeCachePath() (path string) {
	_user, err := user.Current()
	if err != nil {
		log.Print(err)
		return
	}
	path = filepath.Join(_user.HomeDir, ".dms-ffprobe-cache")
	return
}

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

	path := flag.String("path", config.Path, "browse root path")
	ifName := flag.String("ifname", config.IfName, "specific SSDP network interface")
	http := flag.String("http", config.Http, "http server port")
	friendlyName := flag.String("friendlyName", config.FriendlyName, "server friendly name")
	logHeaders := flag.Bool("logHeaders", config.LogHeaders, "log HTTP headers")
	fFprobeCachePath := flag.String("fFprobeCachePath", config.FFprobeCachePath, "path to FFprobe cache file")
	configFilePath := flag.String("config", "", "json configuration file")
	flag.BoolVar(&config.NoTranscode, "noTranscode", false, "disable transcoding")
	flag.BoolVar(&config.StallEventSubscribe, "stallEventSubscribe", false, "workaround for some bad event subscribers")

	flag.Parse()
	if flag.NArg() != 0 {
		flag.Usage()
		log.Fatalf("%s: %s\n", "unexpected positional arguments", flag.Args())
	}

	config.Path = *path
	config.IfName = *ifName
	config.Http = *http
	config.FriendlyName = *friendlyName
	config.LogHeaders = *logHeaders
	config.FFprobeCachePath = *fFprobeCachePath

	if len(*configFilePath) > 0 {
		config.load(*configFilePath)
	}

	cache := &fFprobeCache{
		c: rrcache.New(64 << 20),
	}
	if err := cache.load(config.FFprobeCachePath); err != nil {
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
			var tmp []net.Interface
			for _, if_ := range ifs {
				if if_.Flags&net.FlagUp == 0 || if_.MTU <= 0 {
					continue
				}
				tmp = append(tmp, if_)
			}
			ifs = tmp
			return
		}(config.IfName),
		HTTPConn: func() net.Listener {
			conn, err := net.Listen("tcp", config.Http)
			if err != nil {
				log.Fatal(err)
			}
			return conn
		}(),
		FriendlyName:   config.FriendlyName,
		RootObjectPath: filepath.Clean(config.Path),
		FFProbeCache:   cache,
		LogHeaders:     config.LogHeaders,
		NoTranscode:    config.NoTranscode,
		Icons: []dms.Icon{
			dms.Icon{
				Width:      48,
				Height:     48,
				Depth:      8,
				Mimetype:   "image/png",
				ReadSeeker: bytes.NewReader(MustAsset("data/VGC Sonic.png")),
			},
			dms.Icon{
				Width:      128,
				Height:     128,
				Depth:      8,
				Mimetype:   "image/png",
				ReadSeeker: bytes.NewReader(MustAsset("data/VGC Sonic 128.png")),
			},
		},
		StallEventSubscribe: config.StallEventSubscribe,
	}
	go func() {
		if err := dmsServer.Serve(); err != nil {
			log.Fatal(err)
		}
	}()
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, os.Interrupt, syscall.SIGTERM)
	<-sigs
	err := dmsServer.Close()
	if err != nil {
		log.Fatal(err)
	}
	if err := cache.save(config.FFprobeCachePath); err != nil {
		log.Print(err)
	}
}

func (cache *fFprobeCache) load(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	dec := json.NewDecoder(f)
	var items []dms.FfprobeCacheItem
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

func (cache *fFprobeCache) save(path string) error {
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
