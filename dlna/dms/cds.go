package dms

import (
	"encoding/xml"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"bitbucket.org/anacrolix/dms/misc"

	"bitbucket.org/anacrolix/dms/dlna"
	"bitbucket.org/anacrolix/dms/ffmpeg"
	"bitbucket.org/anacrolix/dms/futures"
	"bitbucket.org/anacrolix/dms/upnp"
	"bitbucket.org/anacrolix/dms/upnpav"
)

type contentDirectoryService struct {
	*Server
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

// Turns the given entry and DMS host into a UPnP object. A nil object is
// returned if the entry is not of interest.
func (me *contentDirectoryService) entryObject(entry cdsEntry, host string) interface{} {
	obj := upnpav.Object{
		ID:         entry.object.ID(),
		Restricted: 1,
		ParentID:   entry.object.ParentID(),
	}
	if entry.FileInfo.IsDir() {
		obj.Class = "object.container.storageFolder"
		obj.Title = entry.FileInfo.Name()
		return upnpav.Container{
			Object:     obj,
			ChildCount: entry.object.ChildCount(),
		}
	}
	entryFilePath := entry.object.FilePath()
	mimeType := mimeTypeByPath(entryFilePath)
	mimeTypeType := mimeType.Type()
	if !mimeTypeType.IsMedia() {
		return nil
	}
	iconURI := (&url.URL{
		Scheme: "http",
		Host:   host,
		Path:   iconPath,
		RawQuery: url.Values{
			"path": {entry.object.Path},
		}.Encode(),
	}).String()
	obj.Icon = iconURI
	// TODO(anacrolix): This might not be necessary due to item res image
	// element.
	obj.AlbumArtURI = iconURI
	obj.Class = "object.item." + string(mimeTypeType) + "Item"
	var (
		nativeBitrate uint
		resDuration   string
	)
	ffInfo, probeErr := me.ffmpegProbe(entryFilePath)
	switch probeErr {
	case nil:
		if ffInfo != nil {
			nativeBitrate, _ = ffInfo.Bitrate()
			if d, err := ffInfo.Duration(); err == nil {
				resDuration = misc.FormatDurationSexagesimal(d)
			}
		}
	case ffmpeg.FfprobeUnavailableError:
	default:
		log.Printf("error probing %s: %s", entryFilePath, probeErr)
	}
	if obj.Title == "" {
		obj.Title = entry.FileInfo.Name()
	}
	resolution := func() string {
		if ffInfo != nil {
			for _, strm := range ffInfo.Streams {
				if strm["codec_type"] != "video" {
					continue
				}
				width := strm["width"]
				height := strm["height"]
				return fmt.Sprintf("%.0fx%.0f", width, height)
			}
		}
		return ""
	}()
	item := upnpav.Item{
		Object: obj,
		// Capacity: 1 for raw, 2 for transcode, 1 for icon.
		Res: make([]upnpav.Resource, 0, 4),
	}
	item.Res = append(item.Res, upnpav.Resource{
		URL: (&url.URL{
			Scheme: "http",
			Host:   host,
			Path:   resPath,
			RawQuery: url.Values{
				"path": {entry.object.Path},
			}.Encode(),
		}).String(),
		ProtocolInfo: fmt.Sprintf("http-get:*:%s:%s", mimeType, dlna.ContentFeatures{
			SupportRange: true,
		}.String()),
		Bitrate:    nativeBitrate,
		Duration:   resDuration,
		Size:       uint64(entry.FileInfo.Size()),
		Resolution: resolution,
	})
	if mimeTypeType == "video" {
		item.Res = append(item.Res, transcodeResources(host, entry.object.Path, resolution, resDuration)...)
	}
	if mimeTypeType.IsMedia() {
		item.Res = append(item.Res, upnpav.Resource{
			URL: (&url.URL{
				Scheme: "http",
				Host:   host,
				Path:   iconPath,
				RawQuery: url.Values{
					"path": {entry.object.Path},
					"c":    {"jpeg"},
				}.Encode(),
			}).String(),
			ProtocolInfo: "http-get:*:image/jpeg:DLNA.ORG_PN=JPEG_TN",
		})
	}
	return item
}

func (me *contentDirectoryService) readContainer(o object, host, userAgent string) (ret []interface{}, err error) {
	sfis := sortableFileInfoSlice{
		// TODO(anacrolix): Dig up why this special cast was added.
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
				ret <- cdsEntry{fi, object{path.Join(o.Path, fi.Name()), me.RootObjectPath}}
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

// ContentDirectory object from ObjectID.
func (me *contentDirectoryService) objectFromID(id string) (o object, err error) {
	o.Path, err = url.QueryUnescape(id)
	if err != nil {
		return
	}
	if o.Path == "0" {
		o.Path = "/"
	}
	o.Path = path.Clean(o.Path)
	if !path.IsAbs(o.Path) {
		err = errors.New("bad ObjectID")
		return
	}
	o.RootObjectPath = me.RootObjectPath
	return
}

func (me *contentDirectoryService) Handle(action string, argsXML []byte, r *http.Request) (map[string]string, *upnp.Error) {
	host := r.Host
	userAgent := r.UserAgent()
	switch action {
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
						object:   obj,
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
	return nil, &upnp.InvalidActionError
}

// Represents a ContentDirectory object.
type object struct {
	Path           string // The cleaned, absolute path for the object relative to the server.
	RootObjectPath string
}

// Returns the number of children this object has, such as for a container.
func (me *object) ChildCount() int {
	dir, err := os.Open(me.FilePath())
	if err != nil {
		log.Print(err)
		return 0
	}
	defer dir.Close()
	names, err := dir.Readdirnames(-1)
	if err != nil {
		log.Print(err)
	}
	return len(names)
}

// Returns the actual local filesystem path for the object.
func (o *object) FilePath() string {
	return filepath.Join(o.RootObjectPath, filepath.FromSlash(o.Path))
}

// Returns the ObjectID for the object. This is used in various ContentDirectory actions.
func (o object) ID() string {
	if !path.IsAbs(o.Path) {
		panic(o.Path)
	}
	if len(o.Path) == 1 {
		return "0"
	}
	return url.QueryEscape(o.Path)
}

func (o *object) IsRoot() bool {
	return o.Path == "/"
}

// Returns the object's parent ObjectID. Fortunately it can be deduced from the
// ObjectID (for now).
func (o object) ParentID() string {
	if o.IsRoot() {
		return "-1"
	}
	o.Path = path.Dir(o.Path)
	return o.ID()
}

// TODO: Explain why this function exists rather than just calling os.(*File).Readdir.
func (o *object) readDir() (fis []os.FileInfo, err error) {
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
	fis = make([]os.FileInfo, 0, len(dirContent))
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

// Puts an object with it's previously obtained FileInfo.
type cdsEntry struct {
	os.FileInfo
	object
}

type sortableFileInfoSlice struct {
	fileInfoSlice []os.FileInfo
	FoldersLast   bool
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
