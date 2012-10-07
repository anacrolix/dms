package dms

import (
	"bitbucket.org/anacrolix/dms/dlna"
	"bitbucket.org/anacrolix/dms/ffmpeg"
	"bitbucket.org/anacrolix/dms/futures"
	"bitbucket.org/anacrolix/dms/misc"
	"bitbucket.org/anacrolix/dms/soap"
	"bitbucket.org/anacrolix/dms/ssdp"
	"bitbucket.org/anacrolix/dms/upnp"
	"bitbucket.org/anacrolix/dms/upnpav"
	"bytes"
	"crypto/md5"
	"encoding/xml"
	"fmt"
	"io"
	"log"
	"mime"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/user"
	"path"
	"runtime"
	"sort"
	"strings"
	"syscall"
	"time"
)

const (
	serverField                 = "Linux/3.4 DLNADOC/1.50 UPnP/1.0 DMS/1.0"
	rootDeviceType              = "urn:schemas-upnp-org:device:MediaServer:1"
	rootDeviceModelName         = "dms 1.0"
	resPath                     = "/res"
	rootDescPath                = "/rootDesc.xml"
	contentDirectorySCPDURL     = "/scpd/ContentDirectory.xml"
	contentDirectoryEventSubURL = "/evt/ContentDirectory"
	contentDirectoryControlURL  = "/ctl/ContentDirectory"
)

func makeDeviceUuid(unique string) string {
	h := md5.New()
	if _, err := io.WriteString(h, unique); err != nil {
		panic(err)
	}
	buf := h.Sum(nil)
	return fmt.Sprintf("uuid:%x-%x-%x-%x-%x", buf[:4], buf[4:6], buf[6:8], buf[8:10], buf[10:16])
}

var services = []upnp.Service{
	upnp.Service{
		ServiceType: "urn:schemas-upnp-org:service:ContentDirectory:1",
		ServiceId:   "urn:upnp-org:serviceId:ContentDirectory",
		SCPDURL:     contentDirectorySCPDURL,
		ControlURL:  contentDirectoryControlURL,
		EventSubURL: contentDirectoryEventSubURL,
	},
	/*
		service{
			ServiceType: "urn:schemas-upnp-org:service:ConnectionManager:1",
			ServiceId:   "urn:upnp-org:serviceId:ConnectionManager",
			SCPDURL:     "/scpd/ConnectionManager.xml",
			ControlURL:  "/ctl/ConnectionManager",
		},
	*/
}

func devices() []string {
	return []string{
		"urn:schemas-upnp-org:device:MediaServer:1",
	}
}

func serviceTypes() (ret []string) {
	for _, s := range services {
		ret = append(ret, s.ServiceType)
	}
	return
}
func (me *server) httpPort() int {
	return me.httpConn.Addr().(*net.TCPAddr).Port
}

func (me *server) serveHTTP() {
	srv := &http.Server{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Ext", "")
			w.Header().Set("Server", serverField)
			me.httpServeMux.ServeHTTP(w, r)
		}),
	}
	err := srv.Serve(me.httpConn)
	log.Fatalln(err)
}

func (me *server) doSSDP() {
	active := 0
	stopped := make(chan struct{})
	ifs, err := net.Interfaces()
	if err != nil {
		log.Fatalln("error enumerating interfaces:", err)
	}
	for _, if_ := range ifs {
		s := ssdp.Server{
			Interface: if_,
			Devices:   devices(),
			Services:  serviceTypes(),
			Location: func(ip net.IP) string {
				return me.location(ip)
			},
			Server:         serverField,
			UUID:           me.rootDeviceUUID,
			NotifyInterval: 30,
		}
		active++
		go func(if_ net.Interface) {
			if err := s.Init(); err != nil {
				log.Println(if_.Name, err)
			} else {
				log.Println("started SSDP on", if_.Name)
				if err := s.Serve(); err != nil {
					log.Printf("%q: %q\n", if_.Name, err)
				}
				s.Close()
			}
			stopped <- struct{}{}
		}(if_)
	}
	for active > 0 {
		<-stopped
		active--
	}
	log.Fatalln("no interfaces remain")
}

var (
	startTime time.Time
)

type server struct {
	httpConn       *net.TCPListener
	httpServeMux   *http.ServeMux
	rootObjectPath string
	friendlyName   string
	rootDescXML    []byte
	rootDeviceUUID string
	logger         *log.Logger
}

func (me *server) childCount(path_ string) int {
	f, err := os.Open(path_)
	if err != nil {
		me.logger.Println(err)
		return 0
	}
	defer f.Close()
	fis, err := f.Readdir(-1)
	if err != nil {
		me.logger.Println(err)
		return 0
	}
	ret := 0
	for _, fi := range fis {
		ret += len(fileEntries(fi, path_))
	}
	return ret
}

// update the UPnP object fields from ffprobe data
// priority is given the format section, and then the streams sequentially
func itemExtra(item *upnpav.Object, info *ffmpeg.Info) {
	setFromTags := func(m map[string]string) {
		for key, val := range m {
			setIfUnset := func(s *string) {
				if *s == "" {
					*s = val
				}
			}
			switch strings.ToLower(key) {
			case "tag:artist":
				setIfUnset(&item.Artist)
			case "tag:album":
				setIfUnset(&item.Album)
			case "tag:genre":
				setIfUnset(&item.Genre)
			}
		}
	}
	setFromTags(info.Format)
	for _, m := range info.Streams {
		setFromTags(m)
	}
}

// returns res attributes for the raw stream
func (me *server) itemResExtra(info *ffmpeg.Info) (bitrate uint, duration string) {
	fmt.Sscan(info.Format["bit_rate"], &bitrate)
	if d := info.Format["duration"]; d != "" && d != "N/A" {
		var f float64
		_, err := fmt.Sscan(info.Format["duration"], &f)
		if err != nil {
			me.logger.Println(err)
		} else {
			duration = misc.FormatDurationSexagesimal(time.Duration(f * float64(time.Second)))
		}
	}
	return
}

func (me *server) entryObject(parentID, host string, entry CDSEntry) interface{} {
	path_ := path.Join(entry.ParentPath, entry.FileInfo.Name())
	obj := upnpav.Object{
		ID:         path_,
		Title:      entry.Title,
		Restricted: 1,
		ParentID:   parentID,
	}
	if entry.FileInfo.IsDir() {
		obj.Class = "object.container.storageFolder"
		return upnpav.Container{
			Object:     obj,
			ChildCount: me.childCount(path_),
		}
	}
	mimeType := func() string {
		if entry.Transcode {
			return "video/mpeg"
		}
		return mime.TypeByExtension(path.Ext(entry.FileInfo.Name()))
	}()
	obj.Class = "object.item." + strings.SplitN(mimeType, "/", 2)[0] + "Item"
	values := url.Values{}
	values.Set("path", path_)
	if entry.Transcode {
		values.Set("transcode", "t")
	}
	url_ := &url.URL{
		Scheme:   "http",
		Host:     host,
		Path:     resPath,
		RawQuery: values.Encode(),
	}
	cf := dlna.ContentFeatures{}
	if entry.Transcode {
		cf.SupportTimeSeek = true
		cf.Transcoded = true
	} else {
		cf.SupportRange = true
	}
	mainRes := upnpav.Resource{
		ProtocolInfo: "http-get:*:" + mimeType + ":" + cf.String(),
		URL:          url_.String(),
		Size:         uint64(entry.FileInfo.Size()),
	}
	ffInfo, err := ffmpeg.Probe(path_)
	if err != nil {
		me.logger.Printf("error probing %s: %s", path_, err)
	}
	if ffInfo != nil {
		itemExtra(&obj, ffInfo)
		mainRes.Bitrate, mainRes.Duration = me.itemResExtra(ffInfo)
	}
	return upnpav.Item{
		Object: obj,
		Res:    []upnpav.Resource{mainRes},
	}
}

func fileEntries(fileInfo os.FileInfo, parentPath string) []CDSEntry {
	if fileInfo.IsDir() {
		return []CDSEntry{
			{fileInfo.Name(), fileInfo, parentPath, false},
		}
	}
	mimeType := mime.TypeByExtension(path.Ext(fileInfo.Name()))
	mimeTypeType := strings.SplitN(mimeType, "/", 2)[0]
	ret := []CDSEntry{
		{fileInfo.Name(), fileInfo, parentPath, false},
	}
	if mimeTypeType == "video" {
		ret = append(ret, CDSEntry{
			fileInfo.Name() + "/transcode",
			fileInfo,
			parentPath,
			true,
		})
	}
	return ret
}

type CDSEntry struct {
	Title      string
	FileInfo   os.FileInfo
	ParentPath string
	Transcode  bool
}

type FileInfoSlice []os.FileInfo

func (me FileInfoSlice) Len() int {
	return len(me)
}

func (me FileInfoSlice) Less(i, j int) bool {
	return strings.ToLower(me[i].Name()) < strings.ToLower(me[j].Name())
}

func (me FileInfoSlice) Swap(i, j int) {
	me[i], me[j] = me[j], me[i]
}

func (me *server) ReadContainer(path_, parentID, host string) (ret []interface{}) {
	dir, err := os.Open(path_)
	if err != nil {
		me.logger.Println(err)
		return
	}
	defer dir.Close()
	var fis FileInfoSlice
	fis, err = dir.Readdir(-1)
	if err != nil {
		panic(err)
	}
	sort.Sort(fis)
	pool := futures.NewExecutor(runtime.NumCPU())
	defer pool.Shutdown()
	for obj := range pool.Map(func(entry interface{}) interface{} {
		return me.entryObject(parentID, host, entry.(CDSEntry))
	}, func() <-chan interface{} {
		ret := make(chan interface{})
		go func() {
			for _, fi := range fis {
				for _, entry := range fileEntries(fi, path_) {
					ret <- entry
				}
			}
			close(ret)
		}()
		return ret
	}()) {
		ret = append(ret, obj)
	}
	return
}

type Browse struct {
	ObjectID       string
	BrowseFlag     string
	Filter         string
	StartingIndex  int
	RequestedCount int
}

func (me *server) contentDirectoryResponseArgs(sa upnp.SoapAction, argsXML []byte, host string) (map[string]string, *upnp.Error) {
	switch sa.Action {
	case "GetSortCapabilities":
		return map[string]string{
			"SortCaps": "dc:title",
		}, nil
	case "Browse":
		var browse Browse
		if err := xml.Unmarshal([]byte(argsXML), &browse); err != nil {
			panic(err)
		}
		path := browse.ObjectID
		if path == "0" {
			path = me.rootObjectPath
		}
		switch browse.BrowseFlag {
		case "BrowseDirectChildren":
			objs := me.ReadContainer(path, browse.ObjectID, host)
			totalMatches := len(objs)
			objs = objs[browse.StartingIndex:]
			if browse.RequestedCount != 0 && int(browse.RequestedCount) < len(objs) {
				objs = objs[:browse.RequestedCount]
			}
			result, err := xml.MarshalIndent(objs, "", "  ")
			if err != nil {
				panic(err)
			}
			return map[string]string{
				"TotalMatches":   fmt.Sprint(totalMatches),
				"NumberReturned": fmt.Sprint(len(objs)),
				"Result":         didl_lite(string(result)),
				"UpdateID":       "0",
			}, nil
		default:
			me.logger.Println("unhandled browse flag:", browse.BrowseFlag)
		}
	case "GetSearchCapabilities":
		return map[string]string{
			"SearchCaps": "",
		}, nil
	}
	me.logger.Println("unhandled content directory action:", sa.Action)
	return nil, &upnp.InvalidActionError
}

func (me *server) serveDLNATranscode(w http.ResponseWriter, r *http.Request, path_ string) {
	w.Header().Set(dlna.TransferModeDomain, "Streaming")
	w.Header().Set("content-type", "video/mpeg")
	w.Header().Set(dlna.ContentFeaturesDomain, (dlna.ContentFeatures{
		Transcoded:      true,
		SupportTimeSeek: true,
	}).String())
	dlnaRangeHeader := r.Header.Get(dlna.TimeSeekRangeDomain)
	var nptStart, nptLength string
	if dlnaRangeHeader != "" {
		if !strings.HasPrefix(dlnaRangeHeader, "npt=") {
			me.logger.Println("bad range:", dlnaRangeHeader)
			return
		}
		dlnaRange, err := dlna.ParseNPTRange(dlnaRangeHeader[len("npt="):])
		if err != nil {
			me.logger.Println("bad range:", dlnaRangeHeader)
			return
		}
		nptStart = dlna.FormatNPTTime(dlnaRange.Start)
		if dlnaRange.End > 0 {
			nptLength = dlna.FormatNPTTime(dlnaRange.End - dlnaRange.Start)
		}
		// passing an NPT duration seems to cause trouble
		// so just pass the "iono" duration
		w.Header().Set(dlna.TimeSeekRangeDomain, dlnaRangeHeader+"/*")
	}
	p, err := misc.Transcode(path_, nptStart, nptLength)
	if err != nil {
		me.logger.Println(err)
		return
	}
	defer p.Close()
	w.WriteHeader(206)
	if n, err := io.Copy(w, p); err != nil {
		// not an error if the peer closes the connection
		func() {
			if opErr, ok := err.(*net.OpError); ok {
				if opErr.Err == syscall.EPIPE {
					return
				}
			}
			me.logger.Println("after copying", n, "bytes:", err)
		}()
	}
}

func init() {
	startTime = time.Now()
}

func (me *server) SetRootPath(path string) {
	me.rootObjectPath = path
}

func New(path string, logger *log.Logger) (*server, error) {
	server := &server{
		friendlyName: fmt.Sprintf("%s: %s on %s", rootDeviceModelName, func() string {
			user, err := user.Current()
			if err != nil {
				panic(err)
			}
			return user.Name
		}(), func() string {
			name, err := os.Hostname()
			if err != nil {
				panic(err)
			}
			return name
		}()),
		rootObjectPath: path,
		httpServeMux:   http.NewServeMux(),
		logger:         logger,
	}
	server.rootDeviceUUID = makeDeviceUuid(server.friendlyName)
	var err error
	server.rootDescXML, err = xml.MarshalIndent(
		upnp.DeviceDesc{
			SpecVersion: upnp.SpecVersion{Major: 1, Minor: 0},
			Device: upnp.Device{
				DeviceType:   rootDeviceType,
				FriendlyName: server.friendlyName,
				Manufacturer: "Matt Joiner <anacrolix@gmail.com>",
				ModelName:    rootDeviceModelName,
				UDN:          server.rootDeviceUUID,
				ServiceList:  services,
			},
		},
		" ", "  ")
	if err != nil {
		panic(err)
	}
	server.rootDescXML = append([]byte(`<?xml version="1.0"?>`), server.rootDescXML...)
	server.httpConn, err = net.ListenTCP("tcp", &net.TCPAddr{Port: 1338})
	if err != nil {
		return nil, err
	}
	server.logger.Println("HTTP server on", server.httpConn.Addr())
	server.initMux(server.httpServeMux)
	return server, nil
}

func (server *server) initMux(mux *http.ServeMux) {
	mux.HandleFunc("/", func(resp http.ResponseWriter, req *http.Request) {
		resp.Header().Set("content-type", "text/html")
		err := rootTmpl.Execute(resp, struct {
			Readonly bool
			Path     string
		}{
			true,
			server.rootObjectPath,
		})
		if err != nil {
			server.logger.Println(err)
		}
	})
	mux.HandleFunc(contentDirectoryEventSubURL, func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "vlc sux", http.StatusNotImplemented)
	})
	mux.HandleFunc(resPath, func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			server.logger.Println(err)
		}
		path := r.Form.Get("path")
		if r.Form.Get("transcode") == "" {
			http.ServeFile(w, r, path)
			return
		}
		server.serveDLNATranscode(w, r, path)
	})
	mux.HandleFunc(rootDescPath, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("content-type", `text/xml; charset="utf-8"`)
		w.Header().Set("content-length", fmt.Sprint(len(server.rootDescXML)))
		w.Header().Set("server", serverField)
		w.Write(server.rootDescXML)
	})
	mux.HandleFunc(contentDirectorySCPDURL, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("content-type", `text/xml; charset="utf-8"`)
		http.ServeContent(w, r, ".xml", startTime, bytes.NewReader([]byte(ContentDirectoryServiceDescription)))
	})
	mux.HandleFunc(contentDirectoryControlURL, func(w http.ResponseWriter, r *http.Request) {
		soapActionString := r.Header.Get("SOAPACTION")
		soapAction, ok := upnp.ParseActionHTTPHeader(soapActionString)
		if !ok {
			server.logger.Println("invalid soapaction:", soapActionString)
			return
		}
		var env soap.Envelope
		if err := xml.NewDecoder(r.Body).Decode(&env); err != nil {
			panic(err)
		}
		w.Header().Set("Content-Type", `text/xml; charset="utf-8"`)
		w.Header().Set("Ext", "")
		w.Header().Set("Server", serverField)
		actionResponseXML, err := xml.MarshalIndent(func() interface{} {
			argMap, err := server.contentDirectoryResponseArgs(soapAction, env.Body.Action, r.Host)
			if err != nil {
				w.WriteHeader(500)
				return soap.NewFault("UPnPError", err)
			}
			args := make([]soap.Arg, 0, len(argMap))
			for argName, value := range argMap {
				args = append(args, soap.Arg{
					XMLName: xml.Name{Local: argName},
					Value:   value,
				})
			}
			return soap.Action{
				XMLName: xml.Name{
					Space: soapAction.ServiceURN.String(),
					Local: soapAction.Action + "Response",
				},
				Args: args,
			}
		}(), "", "  ")
		if err != nil {
			panic(err)
		}
		body, err := xml.MarshalIndent(soap.NewEnvelope(actionResponseXML), "", "  ")
		if err != nil {
			panic(err)
		}
		body = append([]byte(xml.Header), body...)
		if _, err := w.Write(body); err != nil {
			panic(err)
		}
	})
}

func (me *server) Serve() {
	go me.serveHTTP()
	me.doSSDP()
}

func (me *server) Close() {
	me.httpConn.Close()
}

func didl_lite(chardata string) string {
	return `<DIDL-Lite` +
		` xmlns:dc="http://purl.org/dc/elements/1.1/"` +
		` xmlns:upnp="urn:schemas-upnp-org:metadata-1-0/upnp/"` +
		` xmlns="urn:schemas-upnp-org:metadata-1-0/DIDL-Lite/"` +
		` xmlns:dlna="urn:schemas-dlna-org:metadata-1-0/">` +
		chardata +
		`</DIDL-Lite>`
}

func (me *server) location(ip net.IP) string {
	url := url.URL{
		Scheme: "http",
		Host: (&net.TCPAddr{
			IP:   ip,
			Port: me.httpPort(),
		}).String(),
		Path: rootDescPath,
	}
	return url.String()
}
