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
	"path/filepath"
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
	transcodeMimeType           = "video/mpeg"
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
func (me *Server) httpPort() int {
	return me.HTTPConn.Addr().(*net.TCPAddr).Port
}

func (me *Server) serveHTTP() error {
	srv := &http.Server{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Ext", "")
			w.Header().Set("Server", serverField)
			me.httpServeMux.ServeHTTP(w, r)
		}),
	}
	err := srv.Serve(me.HTTPConn)
	select {
	case <-me.closed:
		return nil
	default:
		return err
	}
}

func (me *Server) doSSDP() {
	active := 0
	stopped := make(chan struct{})
	for _, if_ := range me.Interfaces {
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
				sStopped := make(chan struct{})
				go func() {
					if err := s.Serve(); err != nil {
						log.Printf("%q: %q\n", if_.Name, err)
					}
					close(sStopped)
				}()
				select {
				case <-me.closed:
				case <-sStopped:
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
}

var (
	startTime time.Time
)

type Server struct {
	HTTPConn       net.Listener
	FriendlyName   string
	Interfaces     []net.Interface
	httpServeMux   *http.ServeMux
	RootObjectPath string
	rootDescXML    []byte
	rootDeviceUUID string
	FFProbeCache   Cache
	closed         chan struct{}
	ssdpStopped    chan struct{}
}

type Cache interface {
	Set(key interface{}, value interface{})
	Get(key interface{}) (value interface{}, ok bool)
}

type DummyFFProbeCache struct{}

func (DummyFFProbeCache) Set(interface{}, interface{}) {}

func (DummyFFProbeCache) Get(interface{}) (interface{}, bool) {
	return nil, false
}

type FFprobeCacheItem struct {
	Key   ffmpegInfoCacheKey
	Value *ffmpeg.Info
}

// Represents a ContentDirectory object.
type object struct {
	Path   string  // The cleaned, absolute path for the object relative to the server.
	Server *Server // The object's owning server.
}

// Returns the number of children this object has, such as for a container.
func (me *object) ChildCount() int {
	fis, err := me.readDir()
	if err != nil {
		log.Println(err)
	}
	return len(fis)
}

// Returns the actual local filesystem path for the object.
func (o *object) FilePath() string {
	return filepath.Join(o.Server.RootObjectPath, filepath.FromSlash(o.Path))
}

// Returns the ObjectID for the object. This is used in various ContentDirectory actions.
func (o *object) ID() string {
	switch len(o.Path) {
	case 1:
		return "0"
	default:
		return o.Path
	}
}

// Returns the objects parent ObjectID. Fortunately it can be deduced from the ObjectID (for now).
func (o *object) ParentID() string {
	switch len(o.Path) {
	case 1:
		return "-1"
	default:
		return path.Dir(o.Path)
	}
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
func (me *Server) itemResExtra(info *ffmpeg.Info) (bitrate uint, duration string) {
	fmt.Sscan(info.Format["bit_rate"], &bitrate)
	if d := info.Format["duration"]; d != "" && d != "N/A" {
		var f float64
		_, err := fmt.Sscan(info.Format["duration"], &f)
		if err != nil {
			log.Println(err)
		} else {
			duration = misc.FormatDurationSexagesimal(time.Duration(f * float64(time.Second)))
		}
	}
	return
}

// Example: "video/mpeg"
type MimeType string

// Attempts to guess mime type by peeling off extensions, such as those given
// to incomplete files.
func mimeTypeByBaseName(name string) MimeType {
	for name != "" {
		ext := strings.ToLower(path.Ext(name))
		if ext == "" {
			break
		}
		ret := MimeType(mime.TypeByExtension(ext))
		if ret.Type().IsMedia() {
			return ret
		}
		switch ext {
		case ".part":
			index := strings.LastIndex(name, ".")
			if index >= 0 {
				name = name[:index]
			}
		default:
			return ""
		}
	}
	return ""
}

// Used to determine the MIME-type for the given path
func MimeTypeByPath(path_ string) (ret MimeType) {
	defer func() {
		if ret == "video/x-msvideo" {
			ret = "video/avi"
		}
	}()
	ret = mimeTypeByBaseName(path.Base(path_))
	if ret != "" {
		return
	}
	file, _ := os.Open(path_)
	if file == nil {
		return
	}
	var data [512]byte
	n, _ := file.Read(data[:])
	file.Close()
	ret = MimeType(http.DetectContentType(data[:n]))
	return
}

// Returns the group "type", the part before the '/'.
func (mt MimeType) Type() MimeTypeType {
	return MimeTypeType(strings.SplitN(string(mt), "/", 2)[0])
}

type ffmpegInfoCacheKey struct {
	Path    string
	ModTime int64
}

// Can return nil info with nil err if an earlier Probe gave an error.
func (srv *Server) ffmpegProbe(path string) (info *ffmpeg.Info, err error) {
	// We don't want relative paths in the cache.
	path, err = filepath.Abs(path)
	if err != nil {
		return
	}
	fi, err := os.Stat(path)
	if err != nil {
		return
	}
	key := ffmpegInfoCacheKey{path, fi.ModTime().UnixNano()}
	value, ok := srv.FFProbeCache.Get(key)
	if !ok {
		info, err = ffmpeg.Probe(path)
		err = suppressFFmpegProbeDataErrors(err)
		srv.FFProbeCache.Set(key, info)
		return
	}
	info = value.(*ffmpeg.Info)
	return
}

// Turns the given entry and DMS host into a UPnP object.
func (me *Server) entryObject(entry cdsEntry, host string) interface{} {
	obj := upnpav.Object{
		ID:         entry.Object.ID(),
		Restricted: 1,
		ParentID:   entry.Object.ParentID(),
	}
	if entry.FileInfo.IsDir() {
		obj.Class = "object.container.storageFolder"
		obj.Title = entry.FileInfo.Name()
		return upnpav.Container{
			Object:     obj,
			ChildCount: entry.Object.ChildCount(),
		}
	}
	entryFilePath := entry.Object.FilePath()
	mimeType := MimeTypeByPath(entryFilePath)
	mimeTypeType := mimeType.Type()
	if !mimeTypeType.IsMedia() {
		return nil
	}
	obj.Class = "object.item." + string(mimeTypeType) + "Item"
	var (
		nativeBitrate uint
		duration      string
	)
	ffInfo, probeErr := me.ffmpegProbe(entryFilePath)
	switch probeErr {
	case nil:
		if ffInfo != nil {
			nativeBitrate, duration = me.itemResExtra(ffInfo)
		}
	case ffmpeg.FfprobeUnavailableError:
	default:
		log.Printf("error probing %s: %s", entryFilePath, probeErr)
	}
	if obj.Title == "" {
		obj.Title = entry.FileInfo.Name()
	}
	return upnpav.Item{
		Object: obj,
		Res: func() (ret []upnpav.Resource) {
			ret = append(ret, func() upnpav.Resource {
				return upnpav.Resource{
					URL: (&url.URL{
						Scheme: "http",
						Host:   host,
						Path:   resPath,
						RawQuery: url.Values{
							"path": {entry.Object.Path},
						}.Encode(),
					}).String(),
					ProtocolInfo: fmt.Sprintf("http-get:*:%s:%s", mimeType, dlna.ContentFeatures{
						SupportRange: true,
					}.String()),
					Bitrate:  nativeBitrate,
					Duration: duration,
					Size:     uint64(entry.FileInfo.Size()),
					Resolution: func() string {
						if ffInfo != nil {
							for _, strm := range ffInfo.Streams {
								if strm["codec_type"] != "video" {
									continue
								}
								width := strm["width"]
								height := strm["height"]
								if width != "" && height != "" {
									return fmt.Sprintf("%sx%s", width, height)
								}
							}
						}
						return ""
					}(),
				}
			}())
			if mimeTypeType == "video" {
				ret = append(ret, upnpav.Resource{
					ProtocolInfo: fmt.Sprintf("http-get:*:%s:%s", transcodeMimeType, dlna.ContentFeatures{
						SupportTimeSeek: true,
						Transcoded:      true,
						ProfileName:     "MPEG_PS_PAL",
					}.String()),
					URL: (&url.URL{
						Scheme: "http",
						Host:   host,
						Path:   resPath,
						RawQuery: url.Values{
							"path":      {entry.Object.Path},
							"transcode": {"t"},
						}.Encode(),
					}).String(),
					Resolution: "720x576",
				},
				)
			}
			return
		}(),
	}
}

// The part of a MIME type before the '/'.
type MimeTypeType string

// Returns true if the type is typical media.
func (mtt MimeTypeType) IsMedia() bool {
	switch mtt {
	case "video", "audio":
		return true
	default:
		return false
	}
}

// A content directory service entry contains sufficient information to determine how many entries to each actual file.
type cdsEntry struct {
	FileInfo os.FileInfo // file type. names would do but it's cheaper to do this upfront.
	Object   object
}

type fileInfoSlice []os.FileInfo
type sortableFileInfoSlice struct {
	fileInfoSlice
	FoldersLast bool
}

func (me sortableFileInfoSlice) Len() int {
	return len(me.fileInfoSlice)
}

func (me sortableFileInfoSlice) Less(i, j int) bool {
	if me.fileInfoSlice[i].IsDir() && !me.fileInfoSlice[j].IsDir() {
		return !me.FoldersLast
	}
	if !me.fileInfoSlice[i].IsDir() && me.fileInfoSlice[j].IsDir() {
		return me.FoldersLast
	}
	return strings.ToLower(me.fileInfoSlice[i].Name()) < strings.ToLower(me.fileInfoSlice[j].Name())
}

func (me sortableFileInfoSlice) Swap(i, j int) {
	me.fileInfoSlice[i], me.fileInfoSlice[j] = me.fileInfoSlice[j], me.fileInfoSlice[i]
}

// TODO: Explain why this function exists rather than just calling os.(*File).Readdir.
func (o *object) readDir() (fis fileInfoSlice, err error) {
	dirPath := o.FilePath()
	dirFile, err := os.Open(dirPath)
	if err != nil {
		return
	}
	defer dirFile.Close()
	var dirContent []string
	dirContent, err = dirFile.Readdirnames(-1)
	if err != nil {
		return
	}
	fis = make(fileInfoSlice, 0, len(dirContent))
	for _, file := range dirContent {
		fi, err := os.Stat(filepath.Join(dirPath, file))
		if err != nil {
			log.Print(err)
			continue
		}
		fis = append(fis, fi)
	}
	return
}

func (me *Server) readContainer(o object, host, userAgent string) (ret []interface{}, err error) {
	sfis := sortableFileInfoSlice{
		FoldersLast: strings.Contains(userAgent, `AwoX/1.1`),
	}
	sfis.fileInfoSlice, err = o.readDir()
	if err != nil {
		return
	}
	sort.Sort(sfis)
	pool := futures.NewExecutor(runtime.NumCPU())
	defer pool.Shutdown()
	for obj := range pool.Map(func(entry interface{}) interface{} {
		return me.entryObject(entry.(cdsEntry), host)
	}, func() <-chan interface{} {
		ret := make(chan interface{})
		go func() {
			for _, fi := range sfis.fileInfoSlice {
				ret <- cdsEntry{fi, object{path.Join(o.Path, fi.Name()), me}}
			}
			close(ret)
		}()
		return ret
	}()) {
		if obj != nil {
			ret = append(ret, obj)
		}
	}
	return
}

type browse struct {
	ObjectID       string
	BrowseFlag     string
	Filter         string
	StartingIndex  int
	RequestedCount int
}

// Converts an ContentDirectory ObjectID to the corresponding object path.
func (me *Server) objectIdPath(oid string) (path_ string, err error) {
	switch {
	case oid == "0":
		path_ = "/"
	case len(oid) > 0 && oid[0] == '/':
		path_ = path.Clean(oid)
	default:
		err = fmt.Errorf("invalid ObjectID: %q", oid)
	}
	return
}

// ContentDirectory object from ObjectID.
func (me *Server) objectFromID(id string) (o object, err error) {
	o.Server = me
	o.Path, err = me.objectIdPath(id)
	return
}

func (me *Server) contentDirectoryResponseArgs(sa upnp.SoapAction, argsXML []byte, host, userAgent string) (map[string]string, *upnp.Error) {
	switch sa.Action {
	case "GetSystemUpdateID":
		return map[string]string{
			"Id": fmt.Sprintf("%d", uint32(os.Getpid())),
		}, nil
	case "GetSortCapabilities":
		return map[string]string{
			"SortCaps": "dc:title",
		}, nil
	case "Browse":
		var browse browse
		if err := xml.Unmarshal([]byte(argsXML), &browse); err != nil {
			panic(err)
		}
		obj, err := me.objectFromID(browse.ObjectID)
		if err != nil {
			return nil, &upnp.Error{
				Code: upnpav.NoSuchObjectErrorCode,
				Desc: err.Error(),
			}
		}
		switch browse.BrowseFlag {
		case "BrowseDirectChildren":
			objs, err := me.readContainer(obj, host, userAgent)
			if err != nil {
				return nil, &upnp.Error{
					// readContainer can only fail due to bad ObjectID afaict
					Code: upnpav.NoSuchObjectErrorCode,
					Desc: err.Error(),
				}
			}
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
				"UpdateID":       fmt.Sprintf("%d", uint32(time.Now().Unix())),
			}, nil
		case "BrowseMetadata":
			fileInfo, err := os.Stat(obj.FilePath())
			if err != nil {
				return nil, &upnp.Error{
					Code: upnpav.NoSuchObjectErrorCode,
					Desc: err.Error(),
				}
			}
			return map[string]string{
				"TotalMatches":   "1",
				"NumberReturned": "1",
				"Result": didl_lite(func() string {
					buf, err := xml.MarshalIndent(me.entryObject(cdsEntry{
						FileInfo: fileInfo,
						Object:   obj,
					}, host), "", "  ")
					if err != nil {
						panic(err) // because aliens
					}
					return string(buf)
				}()),
				"UpdateID": fmt.Sprintf("%d", uint32(time.Now().Unix())),
			}, nil
		default:
			return nil, &upnp.Error{
				Code: upnp.ArgumentValueInvalidErrorCode,
				Desc: fmt.Sprint("unhandled browse flag:", browse.BrowseFlag),
			}
		}
	case "GetSearchCapabilities":
		return map[string]string{
			"SearchCaps": "",
		}, nil
	}
	log.Println("unhandled content directory action:", sa.Action)
	return nil, &upnp.InvalidActionError
}

func (me *Server) serveDLNATranscode(w http.ResponseWriter, r *http.Request, path_ string) {
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
			log.Println("bad range:", dlnaRangeHeader)
			return
		}
		dlnaRange, err := dlna.ParseNPTRange(dlnaRangeHeader[len("npt="):])
		if err != nil {
			log.Println("bad range:", dlnaRangeHeader)
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
		log.Println(err)
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
			log.Println("after copying", n, "bytes:", err)
		}()
	}
}

func init() {
	startTime = time.Now()
}

func getDefaultFriendlyName() string {
	return fmt.Sprintf("%s: %s on %s", rootDeviceModelName, func() string {
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
	}())
}

func xmlMarshalOrPanic(value interface{}) []byte {
	ret, err := xml.MarshalIndent(value, "", "  ")
	if err != nil {
		panic(err)
	}
	return ret
}

func (server *Server) initMux(mux *http.ServeMux) {
	mux.HandleFunc("/", func(resp http.ResponseWriter, req *http.Request) {
		resp.Header().Set("content-type", "text/html")
		err := rootTmpl.Execute(resp, struct {
			Readonly bool
			Path     string
		}{
			true,
			server.RootObjectPath,
		})
		if err != nil {
			log.Println(err)
		}
	})
	mux.HandleFunc(contentDirectoryEventSubURL, func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "vlc sux", http.StatusNotImplemented)
	})
	mux.HandleFunc(resPath, func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			// epic fizzle...
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		obj := object{path.Clean("/" + r.Form.Get("path")), server}
		if r.Form.Get("transcode") == "" {
			http.ServeFile(w, r, obj.FilePath())
			return
		}
		server.serveDLNATranscode(w, r, obj.FilePath())
	})
	mux.HandleFunc(rootDescPath, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("content-type", `text/xml; charset="utf-8"`)
		w.Header().Set("content-length", fmt.Sprint(len(server.rootDescXML)))
		w.Header().Set("server", serverField)
		w.Write(server.rootDescXML)
	})
	mux.HandleFunc(contentDirectorySCPDURL, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("content-type", `text/xml; charset="utf-8"`)
		http.ServeContent(w, r, ".xml", startTime, bytes.NewReader([]byte(contentDirectoryServiceDescription)))
	})
	mux.HandleFunc(contentDirectoryControlURL, func(w http.ResponseWriter, r *http.Request) {
		soapActionString := r.Header.Get("SOAPACTION")
		soapAction, ok := upnp.ParseActionHTTPHeader(soapActionString)
		if !ok {
			http.Error(w, fmt.Sprintf("invalid soapaction: %#v", soapActionString), http.StatusBadRequest)
			return
		}
		var env soap.Envelope
		if err := xml.NewDecoder(r.Body).Decode(&env); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		//AwoX/1.1 UPnP/1.0 DLNADOC/1.50
		//log.Println(r.UserAgent())
		w.Header().Set("Content-Type", `text/xml; charset="utf-8"`)
		w.Header().Set("Ext", "")
		w.Header().Set("Server", serverField)
		actionResponseXML, httpResponseStatus := func() ([]byte, int) {
			argMap, err := server.contentDirectoryResponseArgs(soapAction, env.Body.Action, r.Host, r.UserAgent())
			if err != nil {
				return xmlMarshalOrPanic(soap.NewFault("UPnPError", err)), 500
			}
			args := make([]soap.Arg, 0, len(argMap))
			for argName, value := range argMap {
				args = append(args, soap.Arg{
					XMLName: xml.Name{Local: argName},
					Value:   value,
				})
			}
			return []byte(fmt.Sprintf(`<u:%[1]sResponse xmlns:u="%[2]s">%[3]s</u:%[1]sResponse>`, soapAction.Action, soapAction.ServiceURN.String(), xmlMarshalOrPanic(args))), 200
		}()
		bodyStr := fmt.Sprintf(`<?xml version="1.0"?><s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/" s:encodingStyle="http://schemas.xmlsoap.org/soap/encoding/"><s:Body>%s</s:Body></s:Envelope>`, actionResponseXML)
		w.WriteHeader(httpResponseStatus)
		if _, err := w.Write([]byte(bodyStr)); err != nil {
			panic(err)
		}
	})
}

func (srv *Server) Serve() (err error) {
	srv.closed = make(chan struct{})
	if srv.FriendlyName == "" {
		srv.FriendlyName = getDefaultFriendlyName()
	}
	if srv.HTTPConn == nil {
		srv.HTTPConn, err = net.Listen("tcp", "")
		if err != nil {
			return
		}
	}
	if srv.Interfaces == nil {
		srv.Interfaces, err = net.Interfaces()
		if err != nil {
			log.Print(err)
		}
	}
	if srv.FFProbeCache == nil {
		srv.FFProbeCache = DummyFFProbeCache{}
	}
	srv.httpServeMux = http.NewServeMux()
	srv.rootDeviceUUID = makeDeviceUuid(srv.FriendlyName)
	srv.rootDescXML, err = xml.MarshalIndent(
		upnp.DeviceDesc{
			SpecVersion: upnp.SpecVersion{Major: 1, Minor: 0},
			Device: upnp.Device{
				DeviceType:   rootDeviceType,
				FriendlyName: srv.FriendlyName,
				Manufacturer: "Matt Joiner <anacrolix@gmail.com>",
				ModelName:    rootDeviceModelName,
				UDN:          srv.rootDeviceUUID,
				ServiceList:  services,
			},
		},
		" ", "  ")
	if err != nil {
		return
	}
	srv.rootDescXML = append([]byte(`<?xml version="1.0"?>`), srv.rootDescXML...)
	log.Println("HTTP srv on", srv.HTTPConn.Addr())
	srv.initMux(srv.httpServeMux)
	srv.ssdpStopped = make(chan struct{})
	go func() {
		srv.doSSDP()
		close(srv.ssdpStopped)
	}()
	return srv.serveHTTP()
}

func (srv *Server) Close() (err error) {
	close(srv.closed)
	err = srv.HTTPConn.Close()
	<-srv.ssdpStopped
	return
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

func (me *Server) location(ip net.IP) string {
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
