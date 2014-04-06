package dms

import (
	"bitbucket.org/anacrolix/dms/dlna"
	"bitbucket.org/anacrolix/dms/ffmpeg"
	"bitbucket.org/anacrolix/dms/futures"
	"bitbucket.org/anacrolix/dms/misc"
	"bitbucket.org/anacrolix/dms/upnp"
	"bitbucket.org/anacrolix/dms/upnpav"
	"encoding/xml"
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
)

type contentDirectoryService struct {
	*Server
}

// returns res attributes for the raw stream
func (me *contentDirectoryService) itemResExtra(info *ffmpeg.Info) (bitrate uint, duration string) {
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

// Can return nil info with nil err if an earlier Probe gave an error.
func (srv *contentDirectoryService) ffmpegProbe(path string) (info *ffmpeg.Info, err error) {
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
	iconURI := (&url.URL{
		Scheme: "http",
		Host:   host,
		Path:   iconPath,
		RawQuery: url.Values{
			"path": {entry.Object.Path},
		}.Encode(),
	}).String()
	obj := upnpav.Object{
		ID:          entry.Object.ID(),
		Restricted:  1,
		ParentID:    entry.Object.ParentID(),
		Icon:        iconURI,
		AlbumArtURI: iconURI,
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
	mimeType := mimeTypeByPath(entryFilePath)
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
	resolution := func() string {
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
	}()
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
					Bitrate:    nativeBitrate,
					Duration:   duration,
					Size:       uint64(entry.FileInfo.Size()),
					Resolution: resolution,
				}
			}())
			if mimeTypeType == "video" {
				ret = append(ret, transcodeResources(host, entry.Object.Path, resolution, duration)...)
			}
			return
		}(),
	}
}

func (me *contentDirectoryService) readContainer(o object, host, userAgent string) (ret []interface{}, err error) {
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

// Converts an ContentDirectory ObjectID to the corresponding object path.
func (me *contentDirectoryService) objectIdPath(oid string) (path_ string, err error) {
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
func (me *contentDirectoryService) objectFromID(id string) (o object, err error) {
	o.RootObjectPath = me.RootObjectPath
	o.Path, err = me.objectIdPath(id)
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
	return nil, &upnp.InvalidActionError
}
