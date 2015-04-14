package dms

import (
	"bytes"
	"crypto/md5"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"log"
	"mime"
	"net"
	"net/http"
	"net/http/pprof"
	"net/url"
	"os"
	"os/exec"
	"os/user"
	"path"
	"path/filepath"
	"strings"
	"time"

	"bitbucket.org/anacrolix/dms/dlna"
	"bitbucket.org/anacrolix/dms/ffmpeg"
	"bitbucket.org/anacrolix/dms/soap"
	"bitbucket.org/anacrolix/dms/ssdp"
	"bitbucket.org/anacrolix/dms/transcode"
	"bitbucket.org/anacrolix/dms/upnp"
	"bitbucket.org/anacrolix/dms/upnpav"
)

const (
	serverField                 = "Linux/3.4 DLNADOC/1.50 UPnP/1.0 DMS/1.0"
	rootDeviceType              = "urn:schemas-upnp-org:device:MediaServer:1"
	rootDeviceModelName         = "dms 1.0"
	resPath                     = "/res"
	iconPath                    = "/icon"
	rootDescPath                = "/rootDesc.xml"
	contentDirectorySCPDURL     = "/scpd/ContentDirectory.xml"
	contentDirectoryEventSubURL = "/evt/ContentDirectory"
	serviceControlURL           = "/ctl"
)

type transcodeSpec struct {
	mimeType        string
	DLNAProfileName string
	Transcode       func(path string, start, length time.Duration, stderr io.Writer) (r io.ReadCloser, err error)
}

var transcodes = map[string]transcodeSpec{
	"t": {
		mimeType:        "video/mpeg",
		DLNAProfileName: "MPEG_PS_PAL",
		Transcode:       transcode.Transcode,
	},
	"vp8":        {mimeType: "video/webm", Transcode: transcode.VP8Transcode},
	"chromecast": {mimeType: "video/x-matroska", Transcode: transcode.ChromecastTranscode},
}

func makeDeviceUuid(unique string) string {
	h := md5.New()
	if _, err := io.WriteString(h, unique); err != nil {
		panic(err)
	}
	buf := h.Sum(nil)
	return upnp.FormatUUID(buf)
}

// Groups the service definition with its XML description.
type service struct {
	upnp.Service
	SCPD string
}

// Exposed UPnP AV services.
var services = []*service{
	{
		Service: upnp.Service{
			ServiceType: "urn:schemas-upnp-org:service:ContentDirectory:1",
			ServiceId:   "urn:upnp-org:serviceId:ContentDirectory",
			EventSubURL: contentDirectoryEventSubURL,
		},
		SCPD: contentDirectoryServiceDescription,
	},
	// {
	// 	Service: upnp.Service{
	// 		ServiceType: "urn:schemas-upnp-org:service:ConnectionManager:3",
	// 		ServiceId:   "urn:upnp-org:serviceId:ConnectionManager",
	// 	},
	// 	SCPD: connectionManagerServiceDesc,
	// },
}

// The control URL for every service is the same. We're able to infer the desired service from the request headers.
func init() {
	for _, s := range services {
		s.ControlURL = serviceControlURL
	}
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
			if me.LogHeaders {
				fmt.Fprintf(os.Stderr, "%s %s\r\n", r.Method, r.RequestURI)
				r.Header.Write(os.Stderr)
				fmt.Fprintln(os.Stderr)
			}
			w.Header().Set("Ext", "")
			w.Header().Set("Server", serverField)
			me.httpServeMux.ServeHTTP(&mitmRespWriter{
				ResponseWriter: w,
				logHeader:      me.LogHeaders,
			}, r)
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

// An interface with these flags should be valid for SSDP.
const ssdpInterfaceFlags = net.FlagUp | net.FlagMulticast

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
				if if_.Flags&ssdpInterfaceFlags == ssdpInterfaceFlags {
					log.Printf("error creating ssdp server on %s: %s", if_.Name, err)
				}
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
	// The service SOAP handler keyed by service URN.
	services   map[string]UPnPService
	LogHeaders bool
	// Disable transcoding, and the resource elements implied in the CDS.
	NoTranscode bool
}

// UPnP SOAP service.
type UPnPService interface {
	Handle(action string, argsXML []byte, r *http.Request) (respArgs map[string]string, err *upnp.Error)
}

type Cache interface {
	Set(key interface{}, value interface{})
	Get(key interface{}) (value interface{}, ok bool)
}

type dummyFFProbeCache struct{}

func (dummyFFProbeCache) Set(interface{}, interface{}) {}

func (dummyFFProbeCache) Get(interface{}) (interface{}, bool) {
	return nil, false
}

// Public definition so that external modules can persist cache contents.
type FfprobeCacheItem struct {
	Key   ffmpegInfoCacheKey
	Value *ffmpeg.Info
}

// update the UPnP object fields from ffprobe data
// priority is given the format section, and then the streams sequentially
func itemExtra(item *upnpav.Object, info *ffmpeg.Info) {
	setFromTags := func(m map[string]interface{}) {
		for key, val := range m {
			setIfUnset := func(s *string) {
				if *s == "" {
					*s = val.(string)
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

func init() {
	if err := mime.AddExtensionType(".rmvb", "application/vnd.rn-realmedia-vbr"); err != nil {
		panic(err)
	}
}

// Example: "video/mpeg"
type mimeType string

func (me mimeType) IsMedia() bool {
	if me == "application/vnd.rn-realmedia-vbr" {
		return true
	}
	return me.Type().IsMedia()
}

// Attempts to guess mime type by peeling off extensions, such as those given
// to incomplete files. TODO: This function may be misleading, since it
// ignores non-media mime-types in processing.
func mimeTypeByBaseName(name string) mimeType {
	for name != "" {
		ext := strings.ToLower(path.Ext(name))
		if ext == "" {
			break
		}
		ret := mimeType(mime.TypeByExtension(ext))
		if ret.IsMedia() {
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
func MimeTypeByPath(path_ string) (ret mimeType) {
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
	ret = mimeType(http.DetectContentType(data[:n]))
	return
}

// Returns the group "type", the part before the '/'.
func (mt mimeType) Type() mimeTypeType {
	return mimeTypeType(strings.SplitN(string(mt), "/", 2)[0])
}

type ffmpegInfoCacheKey struct {
	Path    string
	ModTime int64
}

func transcodeResources(host, path, resolution, duration string) (ret []upnpav.Resource) {
	ret = make([]upnpav.Resource, 0, len(transcodes))
	for k, v := range transcodes {
		ret = append(ret, upnpav.Resource{
			ProtocolInfo: fmt.Sprintf("http-get:*:%s:%s", v.mimeType, dlna.ContentFeatures{
				SupportTimeSeek: true,
				Transcoded:      true,
				ProfileName:     v.DLNAProfileName,
			}.String()),
			URL: (&url.URL{
				Scheme: "http",
				Host:   host,
				Path:   resPath,
				RawQuery: url.Values{
					"path":      {path},
					"transcode": {k},
				}.Encode(),
			}).String(),
			Resolution: resolution,
			Duration:   duration,
		})
	}
	return
}

// The part of a MIME type before the '/'.
type mimeTypeType string

// Returns true if the type is typical media.
func (mtt mimeTypeType) IsMedia() bool {
	switch mtt {
	case "video", "audio":
		return true
	default:
		return false
	}
}

func parseDLNARangeHeader(val string) (ret dlna.NPTRange, err error) {
	if !strings.HasPrefix(val, "npt=") {
		err = errors.New("bad prefix")
		return
	}
	ret, err = dlna.ParseNPTRange(val[len("npt="):])
	if err != nil {
		return
	}
	return
}

// Determines the time-based range to transcode, and sets the appropriate
// headers. Returns !ok if there was an error and the caller should stop
// handling the request.
func handleDLNARange(w http.ResponseWriter, hs http.Header) (r dlna.NPTRange, partialResponse, ok bool) {
	if len(hs[http.CanonicalHeaderKey(dlna.TimeSeekRangeDomain)]) == 0 {
		ok = true
		return
	}
	partialResponse = true
	h := hs.Get(dlna.TimeSeekRangeDomain)
	r, err := parseDLNARangeHeader(h)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	// Passing an exact NPT duration seems to cause trouble pass the "iono"
	// (*) duration instead.
	//
	// TODO: Check that the request range can't already have /.
	w.Header().Set(dlna.TimeSeekRangeDomain, h+"/*")
	ok = true
	return
}

func (me *Server) serveDLNATranscode(w http.ResponseWriter, r *http.Request, path_ string, ts transcodeSpec, tsname string) {
	w.Header().Set(dlna.TransferModeDomain, "Streaming")
	w.Header().Set("content-type", ts.mimeType)
	w.Header().Set(dlna.ContentFeaturesDomain, (dlna.ContentFeatures{
		Transcoded:      true,
		SupportTimeSeek: true,
	}).String())
	// If a range of any kind is given, we have to respond with 206 if we're
	// interpreting that range. Since only the DLNA range is handled in this
	// function, it alone determines if we'll give a partial response.
	range_, partialResponse, ok := handleDLNARange(w, r.Header)
	if !ok {
		return
	}
	ffInfo, _ := me.ffmpegProbe(path_)
	if ffInfo != nil {
		if duration, err := ffInfo.Duration(); err == nil {
			s := fmt.Sprintf("%f", duration.Seconds())
			w.Header().Set("content-duration", s)
			w.Header().Set("x-content-duration", s)
		}
	}
	stderrPath := func() string {
		u, _ := user.Current()
		return filepath.Join(u.HomeDir, ".dms", "log", tsname, filepath.Base(path_))
	}()
	os.MkdirAll(filepath.Dir(stderrPath), 0750)
	logFile, err := os.Create(stderrPath)
	if err != nil {
		log.Printf("couldn't create transcode log file: %s", err)
	} else {
		defer logFile.Close()
		log.Printf("logging transcode to %q", stderrPath)
	}
	p, err := ts.Transcode(path_, range_.Start, range_.End-range_.Start, logFile)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer p.Close()
	// I recently switched this to returning 200 if no range is specified for
	// pure UPnP clients. It's possible that DLNA clients will *always* expect
	// 206. It appears the HTTP standard requires that 206 only be used if a
	// response is not interpreting any range headers.
	w.WriteHeader(func() int {
		if partialResponse {
			return http.StatusPartialContent
		} else {
			return http.StatusOK
		}
	}())
	io.Copy(w, p)
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

// TODO: Document the use of this for debugging.
type mitmRespWriter struct {
	http.ResponseWriter
	loggedHeader bool
	logHeader    bool
}

func (me *mitmRespWriter) WriteHeader(code int) {
	me.doLogHeader(code)
	me.ResponseWriter.WriteHeader(code)
}

func (me *mitmRespWriter) doLogHeader(code int) {
	if !me.logHeader {
		return
	}
	fmt.Fprintln(os.Stderr, code)
	for k, v := range me.Header() {
		fmt.Fprintln(os.Stderr, k, v)
	}
	fmt.Fprintln(os.Stderr)
	me.loggedHeader = true
}

func (me *mitmRespWriter) Write(b []byte) (int, error) {
	if !me.loggedHeader {
		me.doLogHeader(200)
	}
	return me.ResponseWriter.Write(b)
}

// Set the SCPD serve paths.
func init() {
	for _, s := range services {
		p := path.Join("/scpd", s.ServiceId)
		s.SCPDURL = p
	}
}

// Install handlers to serve SCPD for each UPnP service.
func handleSCPDs(mux *http.ServeMux) {
	for _, s := range services {
		mux.HandleFunc(s.SCPDURL, func(serviceDesc string) http.HandlerFunc {
			return func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("content-type", `text/xml; charset="utf-8"`)
				http.ServeContent(w, r, ".xml", startTime, bytes.NewReader([]byte(serviceDesc)))
			}
		}(s.SCPD))
	}
}

// Marshal SOAP response arguments into a response XML snippet.
func marshalSOAPResponse(sa upnp.SoapAction, args map[string]string) []byte {
	soapArgs := make([]soap.Arg, 0, len(args))
	for argName, value := range args {
		soapArgs = append(soapArgs, soap.Arg{
			XMLName: xml.Name{Local: argName},
			Value:   value,
		})
	}
	return []byte(fmt.Sprintf(`<u:%[1]sResponse xmlns:u="%[2]s">%[3]s</u:%[1]sResponse>`, sa.Action, sa.ServiceURN.String(), xmlMarshalOrPanic(soapArgs)))
}

// Handle a SOAP request and return the response arguments or UPnP error.
func (me *Server) soapActionResponse(sa upnp.SoapAction, actionRequestXML []byte, r *http.Request) (map[string]string, *upnp.Error) {
	service, ok := me.services[sa.Type]
	if !ok {
		// TODO: What's the invalid service error?!
		return nil, &upnp.InvalidActionError
	}
	return service.Handle(sa.Action, actionRequestXML, r)
}

// Handle a service control HTTP request.
func (me *Server) serviceControlHandler(w http.ResponseWriter, r *http.Request) {
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
	soapRespXML, code := func() ([]byte, int) {
		respArgs, err := me.soapActionResponse(soapAction, env.Body.Action, r)
		if err != nil {
			return xmlMarshalOrPanic(soap.NewFault("UPnPError", err)), 500
		}
		return marshalSOAPResponse(soapAction, respArgs), 200
	}()
	bodyStr := fmt.Sprintf(`<?xml version="1.0" encoding="utf-8" standalone="yes"?><s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/" s:encodingStyle="http://schemas.xmlsoap.org/soap/encoding/"><s:Body>%s</s:Body></s:Envelope>`, soapRespXML)
	w.WriteHeader(code)
	if _, err := w.Write([]byte(bodyStr)); err != nil {
		panic(err)
	}
}

func safeFilePath(root, given string) string {
	return filepath.Join(root, filepath.FromSlash(path.Clean("/" + given))[1:])
}

func (s *Server) filePath(_path string) string {
	return safeFilePath(s.RootObjectPath, _path)
}

func (me *Server) serveIcon(w http.ResponseWriter, r *http.Request) {
	filePath := me.filePath(r.URL.Query().Get("path"))
	c := r.URL.Query().Get("c")
	if c == "" {
		c = "png"
	}
	cmd := exec.Command("ffmpegthumbnailer", "-i", filePath, "-o", "/dev/stdout", "-c"+c)
	// cmd.Stderr = os.Stderr
	body, err := cmd.Output()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	http.ServeContent(w, r, "", time.Now(), bytes.NewReader(body))
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
		// Without handling this with StatusNotImplemented, IIRC VLC doesn't
		// work correctly.
		http.Error(w, "vlc sux", http.StatusNotImplemented)
	})
	mux.HandleFunc(iconPath, server.serveIcon)
	mux.HandleFunc(resPath, func(w http.ResponseWriter, r *http.Request) {
		filePath := server.filePath(r.URL.Query().Get("path"))
		k := r.URL.Query().Get("transcode")
		if k == "" {
			http.ServeFile(w, r, filePath)
			return
		}
		if server.NoTranscode {
			http.Error(w, "transcodes disabled", http.StatusNotFound)
			return
		}
		spec, ok := transcodes[k]
		if !ok {
			http.Error(w, fmt.Sprintf("bad transcode spec key: %s", k), http.StatusBadRequest)
			return
		}
		server.serveDLNATranscode(w, r, filePath, spec, k)
	})
	mux.HandleFunc(rootDescPath, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("content-type", `text/xml; charset="utf-8"`)
		w.Header().Set("content-length", fmt.Sprint(len(server.rootDescXML)))
		w.Header().Set("server", serverField)
		w.Write(server.rootDescXML)
	})
	handleSCPDs(mux)
	mux.HandleFunc(serviceControlURL, server.serviceControlHandler)
	mux.HandleFunc("/debug/pprof/", pprof.Index)
}

func (s *Server) initServices() {
	urn, err := upnp.ParseServiceType(services[0].ServiceType)
	if err != nil {
		panic(err)
	}
	s.services = map[string]UPnPService{
		urn.Type: &contentDirectoryService{s},
	}
}

func (srv *Server) Serve() (err error) {
	srv.initServices()
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
		srv.FFProbeCache = dummyFFProbeCache{}
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
				ServiceList: func() (ss []upnp.Service) {
					for _, s := range services {
						ss = append(ss, s.Service)
					}
					return
				}(),
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
