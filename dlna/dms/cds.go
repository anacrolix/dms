package dms

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io/fs"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"path"
	"sort"
	"strconv"
	"strings"

	"github.com/anacrolix/ffprobe"
	"github.com/anacrolix/log"

	"github.com/anacrolix/dms/dlna"
	"github.com/anacrolix/dms/misc"
	"github.com/anacrolix/dms/upnp"
	"github.com/anacrolix/dms/upnpav"
)

const dmsMetadataSuffix = ".dms.json"

type contentDirectoryService struct {
	*Server
	upnp.Eventing
}

func (cds *contentDirectoryService) updateIDString() string {
	return fmt.Sprintf("%d", uint32(os.Getpid()))
}

type dmsDynamicStreamResource struct {
	// (optional) DLNA profile name to include in the response e.g. MPEG_PS_PAL
	DlnaProfileName string
	// (optional) DLNA.ORG_FLAGS if you need to override the default (8D500000000000000000000000000000)
	DlnaFlags string
	// required: mime type, e.g. video/mpeg
	MimeType string
	// (optional) resolution, e.g. 640x360
	Resolution string
	// (optional) bitrate, e.g. 721
	Bitrate uint
	// required: OS command to generate this resource on the fly
	Command string
}

type dmsDynamicMediaItem struct {
	// (optional) Title of this media item. Defaults to the filename, if omitted
	Title string
	// (optional) Type of media. Allowed values: "audio", "video". Defaults to video if omitted
	Type string
	// (optional) duration, e.g. 0:21:37.922
	Duration string
	// required: an array of available versions
	Resources []dmsDynamicStreamResource
}

func readDynamicStream(metadataPath string) (*dmsDynamicMediaItem, error) {
	bytes, err := ioutil.ReadFile(metadataPath)
	if err != nil {
		return nil, err
	}
	var re dmsDynamicMediaItem
	err = json.Unmarshal(bytes, &re)
	if err != nil {
		return nil, err
	}
	return &re, nil
}

func (me *contentDirectoryService) cdsObjectDynamicStreamToUpnpavObject(cdsObject object, fileInfo fs.FileInfo, host, userAgent string) (ret interface{}, err error) {
	// at this point we know that entryFilePath points to a .dms.json file; slurp and parse
	dmsMediaItem, err := readDynamicStream(cdsObject.FilePath())
	if err != nil {
		me.Logger.Printf("%s ignored: %v", cdsObject.FilePath(), err)
		return
	}

	obj := upnpav.Object{
		ID:         cdsObject.ID(),
		Restricted: 1,
		ParentID:   cdsObject.ParentID(),
	}
	iconURI := (&url.URL{
		Scheme: "http",
		Host:   host,
		Path:   iconPath,
		RawQuery: url.Values{
			"path": {cdsObject.Path},
		}.Encode(),
	}).String()
	obj.Icon = iconURI
	// TODO(anacrolix): This might not be necessary due to item res image
	// element.
	obj.AlbumArtURI = iconURI

	switch dmsMediaItem.Type {
	case "video":
		obj.Class = "object.item.videoItem"
	case "audio":
		obj.Class = "object.item.audioItem"
	default:
		obj.Class = "object.item.videoItem"
	}

	obj.Title = dmsMediaItem.Title
	if obj.Title == "" {
		obj.Title = strings.TrimSuffix(fileInfo.Name(), dmsMetadataSuffix)
	}
	obj.Date = upnpav.Timestamp{Time: fileInfo.ModTime()}

	item := upnpav.Item{
		Object: obj,
		// Capacity: 1 for icon, plus resources.
		Res: make([]upnpav.Resource, 0, 1+len(dmsMediaItem.Resources)),
	}
	for i, dmsStream := range dmsMediaItem.Resources {
		// default flags borrowed from Serviio: DLNA_ORG_FLAG_SENDER_PACED | DLNA_ORG_FLAG_S0_INCREASE | DLNA_ORG_FLAG_SN_INCREASE | DLNA_ORG_FLAG_STREAMING_TRANSFER_MODE | DLNA_ORG_FLAG_BACKGROUND_TRANSFERT_MODE | DLNA_ORG_FLAG_DLNA_V15
		flags := "8D500000000000000000000000000000"
		if dmsStream.DlnaFlags != "" {
			flags = dmsStream.DlnaFlags
		}
		item.Res = append(item.Res, upnpav.Resource{
			URL: (&url.URL{
				Scheme: "http",
				Host:   host,
				Path:   resPath,
				RawQuery: url.Values{
					"path":  {cdsObject.Path},
					"index": {strconv.Itoa(i)},
				}.Encode(),
			}).String(),
			ProtocolInfo: fmt.Sprintf("http-get:*:%s:%s", dmsStream.MimeType, dlna.ContentFeatures{
				ProfileName:     dmsStream.DlnaProfileName,
				SupportRange:    false,
				SupportTimeSeek: false,
				Transcoded:      true,
				Flags:           flags,
			}.String()),
			Bitrate:    dmsStream.Bitrate,
			Duration:   dmsMediaItem.Duration,
			Resolution: dmsStream.Resolution,
		})
	}

	// and an icon
	item.Res = append(item.Res, upnpav.Resource{
		URL: (&url.URL{
			Scheme: "http",
			Host:   host,
			Path:   iconPath,
			RawQuery: url.Values{
				"path": {cdsObject.Path},
				"c":    {"jpeg"},
			}.Encode(),
		}).String(),
		ProtocolInfo: "http-get:*:image/jpeg:DLNA.ORG_PN=JPEG_TN",
	})

	ret = item
	return
}

// Turns the given entry and DMS host into a UPnP object. A nil object is
// returned if the entry is not of interest.
func (me *contentDirectoryService) cdsObjectToUpnpavObject(
	cdsObject object,
	fileInfo fs.FileInfo,
	host, userAgent string,
) (ret interface{}, err error) {
	entryFilePath := cdsObject.FilePath()
	ignored, err := me.IgnorePath(entryFilePath)
	if err != nil {
		return
	}
	if ignored {
		return
	}
	isDmsMetadata := strings.HasSuffix(entryFilePath, dmsMetadataSuffix)
	if !fileInfo.IsDir() && me.AllowDynamicStreams && isDmsMetadata {
		return me.cdsObjectDynamicStreamToUpnpavObject(cdsObject, fileInfo, host, userAgent)
	}

	obj := upnpav.Object{
		ID:         cdsObject.ID(),
		Restricted: 1,
		ParentID:   cdsObject.ParentID(),
	}
	if fileInfo.IsDir() {
		obj.Class = "object.container.storageFolder"
		obj.Title = fileInfo.Name()
		childCount := me.objectChildCount(cdsObject)
		if childCount != 0 {
			ret = upnpav.Container{Object: obj, ChildCount: childCount}
		}
		return
	}
	if !fileInfo.Mode().IsRegular() {
		me.Logger.Printf("%s ignored: non-regular file", cdsObject.FilePath())
		return
	}
	mimeType, err := MimeTypeByPath(me.FS, entryFilePath)
	if err != nil {
		return
	}
	if !mimeType.IsMedia() {
		if isDmsMetadata {
			me.Logger.Levelf(
				log.Debug,
				"ignored %q: enable support for dynamic streams via the -allowDynamicStreams command line flag", cdsObject.FilePath())
		} else {
			me.Logger.Levelf(log.Debug, "ignored %q: non-media file (%s)", cdsObject.FilePath(), mimeType)
		}
		return
	}
	iconURI := (&url.URL{
		Scheme: "http",
		Host:   host,
		Path:   iconPath,
		RawQuery: url.Values{
			"path": {cdsObject.Path},
		}.Encode(),
	}).String()
	obj.Icon = iconURI
	// TODO(anacrolix): This might not be necessary due to item res image
	// element.
	obj.AlbumArtURI = iconURI
	obj.Class = "object.item." + mimeType.Type() + "Item"
	var (
		ffInfo        *ffprobe.Info
		nativeBitrate uint
		resDuration   string
	)
	if !me.NoProbe {
		ffInfo, probeErr := me.ffmpegProbe(entryFilePath)
		switch probeErr {
		case nil:
			if ffInfo != nil {
				nativeBitrate, _ = ffInfo.Bitrate()
				if d, err := ffInfo.Duration(); err == nil {
					resDuration = misc.FormatDurationSexagesimal(d)
				}
			}
		case ffprobe.ExeNotFound:
		default:
			me.Logger.Printf("error probing %s: %s", entryFilePath, probeErr)
		}
	}
	if obj.Title == "" {
		obj.Title = fileInfo.Name()
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
		// Capacity: 1 for raw, 1 for icon, plus transcodes.
		Res: make([]upnpav.Resource, 0, 2+len(transcodes)),
	}
	item.Res = append(item.Res, upnpav.Resource{
		URL: (&url.URL{
			Scheme: "http",
			Host:   host,
			Path:   resPath,
			RawQuery: url.Values{
				"path": {cdsObject.Path},
			}.Encode(),
		}).String(),
		ProtocolInfo: fmt.Sprintf("http-get:*:%s:%s", mimeType, dlna.ContentFeatures{
			SupportRange: true,
		}.String()),
		Bitrate:    nativeBitrate,
		Duration:   resDuration,
		Size:       uint64(fileInfo.Size()),
		Resolution: resolution,
	})
	if mimeType.IsVideo() {
		if !me.NoTranscode {
			item.Res = append(item.Res, transcodeResources(host, cdsObject.Path, resolution, resDuration)...)
		}
		item.Res = append(item.Res, upnpav.Resource{
			URL: (&url.URL{
				Scheme: "http",
				Host:   host,
				Path:   subtitlePath,
				RawQuery: url.Values{
					"path": {cdsObject.Path},
				}.Encode(),
			}).String(),
			ProtocolInfo: "http-get:*:text/plain",
		})
	}
	if mimeType.IsVideo() || mimeType.IsImage() {
		item.Res = append(item.Res, upnpav.Resource{
			URL: (&url.URL{
				Scheme: "http",
				Host:   host,
				Path:   iconPath,
				RawQuery: url.Values{
					"path": {cdsObject.Path},
					"c":    {"jpeg"},
				}.Encode(),
			}).String(),
			ProtocolInfo: "http-get:*:image/jpeg:DLNA.ORG_PN=JPEG_TN",
		})
	}
	ret = item
	return
}

// Returns all the upnpav objects in a directory.
func (me *contentDirectoryService) readContainer(
	o object,
	host, userAgent string,
) (ret []interface{}, err error) {
	sfis := sortableFileInfoSlice{
		// TODO(anacrolix): Dig up why this special cast was added.
		FoldersLast: strings.Contains(userAgent, `AwoX/1.1`),
	}
	sfis.fileInfoSlice, err = o.readDir(me.FS)
	if err != nil {
		return
	}
	sort.Sort(sfis)
	for _, fi := range sfis.fileInfoSlice {
		child := object{path.Join(o.Path, fi.Name()), me.RootObjectPath}
		obj, err := me.cdsObjectToUpnpavObject(child, fi, host, userAgent)
		if err != nil {
			me.Logger.Printf("error with %s: %s", child.FilePath(), err)
			continue
		}
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
		o.Path = "./"
	}
	o.Path = path.Clean(o.Path)
	o.RootObjectPath = me.RootObjectPath
	return
}

func (me *contentDirectoryService) Handle(action string, argsXML []byte, r *http.Request) ([][2]string, error) {
	host := r.Host
	userAgent := r.UserAgent()
	switch action {
	case "GetSystemUpdateID":
		return [][2]string{
			{"Id", me.updateIDString()},
		}, nil
	case "GetSortCapabilities":
		return [][2]string{
			{"SortCaps", "dc:title"},
		}, nil
	case "Browse":
		var browse browse
		if err := xml.Unmarshal([]byte(argsXML), &browse); err != nil {
			return nil, err
		}
		obj, err := me.objectFromID(browse.ObjectID)
		if err != nil {
			return nil, upnp.Errorf(upnpav.NoSuchObjectErrorCode, "%s", err.Error())
		}
		switch browse.BrowseFlag {
		case "BrowseDirectChildren":
			var objs []interface{}
			if me.OnBrowseDirectChildren == nil {
				objs, err = me.readContainer(obj, host, userAgent)
			} else {
				objs, err = me.OnBrowseDirectChildren(obj.Path, obj.RootObjectPath, host, userAgent)
			}
			if err != nil {
				return nil, upnp.Errorf(upnpav.NoSuchObjectErrorCode, "%s", err.Error())
			}
			totalMatches := len(objs)
			objs = objs[func() (low int) {
				low = browse.StartingIndex
				if low > len(objs) {
					low = len(objs)
				}
				return
			}():]
			if browse.RequestedCount != 0 && int(browse.RequestedCount) < len(objs) {
				objs = objs[:browse.RequestedCount]
			}
			result, err := xml.Marshal(objs)
			if err != nil {
				return nil, err
			}
			return [][2]string{
				{"Result", didl_lite(string(result))},
				{"NumberReturned", fmt.Sprint(len(objs))},
				{"TotalMatches", fmt.Sprint(totalMatches)},
				{"UpdateID", me.updateIDString()},
			}, nil
		case "BrowseMetadata":
			var ret interface{}
			var err error
			if me.OnBrowseMetadata == nil {
				var fileInfo fs.FileInfo
				fileInfo, err = fs.Stat(me.FS, obj.FilePath())
				if err != nil {
					if os.IsNotExist(err) {
						return nil, &upnp.Error{
							Code: upnpav.NoSuchObjectErrorCode,
							Desc: err.Error(),
						}
					}
					return nil, err
				}
				ret, err = me.cdsObjectToUpnpavObject(obj, fileInfo, host, userAgent)
			} else {
				ret, err = me.OnBrowseMetadata(obj.Path, obj.RootObjectPath, host, userAgent)
			}
			if err != nil {
				return nil, err
			}
			buf, err := xml.Marshal(ret)
			if err != nil {
				return nil, err
			}
			return [][2]string{
				{"Result", didl_lite(func() string { return string(buf) }())},
				{"NumberReturned", "1"},
				{"TotalMatches", "1"},
				{"UpdateID", me.updateIDString()},
			}, nil
		default:
			return nil, upnp.Errorf(
				upnp.ArgumentValueInvalidErrorCode,
				"unhandled browse flag: %v",
				browse.BrowseFlag,
			)
		}
	case "GetSearchCapabilities":
		return [][2]string{
			{"SearchCaps", ""},
		}, nil
	// Samsung Extensions
	case "X_GetFeatureList":
		// TODO: make it dependable on model
		// https://github.com/1100101/minidlna/blob/ca6dbba18390ad6f8b8d7b7dbcf797dbfd95e2db/upnpsoap.c#L2153-L2199
		return [][2]string{
			{"FeatureList", `<Features xmlns="urn:schemas-upnp-org:av:avs" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance" xsi:schemaLocation="urn:schemas-upnp-org:av:avs http://www.upnp.org/schemas/av/avs.xsd">
	<Feature name="samsung.com_BASICVIEW" version="1">
		<container id="0" type="object.item.audioItem"/> // "A"
		<container id="0" type="object.item.videoItem"/> // "V"
		<container id="0" type="object.item.imageItem"/> // "I"
	</Feature>
</Features>`},
		}, nil
	case "X_SetBookmark":
		// just ignore
		return [][2]string{}, nil
	default:
		return nil, upnp.InvalidActionError
	}
}

// Represents a ContentDirectory object.
type object struct {
	Path           string // The cleaned, absolute path for the object relative to the server.
	RootObjectPath string
}

func (me *contentDirectoryService) isOfInterest(
	cdsObject object,
	fileInfo fs.FileInfo,
) (ret bool, err error) {
	entryFilePath := cdsObject.FilePath()
	ignored, err := me.IgnorePath(entryFilePath)
	if err != nil {
		return
	}
	if ignored {
		return
	}
	isDmsMetadata := strings.HasSuffix(entryFilePath, dmsMetadataSuffix)
	if !fileInfo.IsDir() && me.AllowDynamicStreams && isDmsMetadata {
		return true, nil
	}

	if fileInfo.IsDir() {
		hasChildren, err := me.objectHasChildren(cdsObject, fileInfo)
		return hasChildren, err
	}
	if !fileInfo.Mode().IsRegular() {
		me.Logger.Printf("%s ignored: non-regular file", cdsObject.FilePath())
		return
	}

	mimeType, err := MimeTypeByPath(me.FS, entryFilePath)
	if err != nil {
		return
	}

	if !mimeType.IsMedia() {
		return
	}
	return true, nil
}

// Returns the number of children this object has, such as for a container.
func (cds *contentDirectoryService) objectChildCount(me object) (count int) {
	fileInfoSlice, err := me.readDir(cds.FS)
	if err != nil {
		return
	}
	for _, fi := range fileInfoSlice {
		child := object{path.Join(me.Path, fi.Name()), cds.RootObjectPath}
		isChild, err := cds.isOfInterest(child, fi)
		if err != nil {
			cds.Logger.Printf("error with %s: %s", child.FilePath(), err)
			continue
		}

		if isChild {
			count++
		}
	}
	return
}

// Returns true if a recursive search for playable items in the provided
// directory succeeds. Returns true on first hit.
func (me *contentDirectoryService) objectHasChildren(
	cdsObject object,
	fileInfo fs.FileInfo,
) (ret bool, err error) {
	if !fileInfo.IsDir() {
		panic("Expected directory")
	}

	files, err := cdsObject.readDir(me.FS)
	if err != nil {
		return
	}
	for _, fi := range files {
		child := object{path.Join(cdsObject.Path, fi.Name()), me.RootObjectPath}
		isCdsObj, err := me.isOfInterest(child, fi)
		if err != nil {
			return false, err
		}
		if isCdsObj {
			// Return on first hit. We don't want a full library scan.
			return true, nil
		}
	}
	return
}

// Returns the actual local filesystem path for the object.
func (o *object) FilePath() string {
	return path.Join(o.RootObjectPath, path.Clean(o.Path))
}

// Returns the ObjectID for the object. This is used in various ContentDirectory actions.
func (o object) ID() string {
	if len(o.Path) == 1 {
		return "0"
	}
	return url.QueryEscape(o.Path)
}

func (o *object) IsRoot() bool {
	return o.Path == "./"
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

// This function exists rather than just calling os.(*File).Readdir because I
// want to stat(), not lstat() each entry.
func (o *object) readDir(fsys fs.FS) (fis []fs.FileInfo, err error) {
	dirFile, err := fs.ReadDir(fsys, o.Path)
	if err != nil {
		return
	}
	fis = make([]fs.FileInfo, 0, len(dirFile))
	for _, file := range dirFile {
		fi, _ := file.Info()
		fis = append(fis, fi)
	}
	return
}

type sortableFileInfoSlice struct {
	fileInfoSlice []fs.FileInfo
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
