package dms

import (
	"mime"
	"net/http"
	"os"
	"path"
	"strings"
)

func init() {
	if err := mime.AddExtensionType(".rmvb", "application/vnd.rn-realmedia-vbr"); err != nil {
		panic(err)
	}
	if err := mime.AddExtensionType(".ogv", "video/ogg"); err != nil {
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

// Returns the group "type", the part before the '/'.
func (mt mimeType) Type() mimeTypeType {
	return mimeTypeType(strings.SplitN(string(mt), "/", 2)[0])
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
	ret, _ = mimeTypeByContent(path_)
	return
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

func mimeTypeByContent(path_ string) (ret mimeType, err error) {
	file, err := os.Open(path_)
	if err != nil {
		return
	}
	defer file.Close()
	var data [512]byte
	n, err := file.Read(data[:])
	if err != nil {
		return
	}
	return mimeType(http.DetectContentType(data[:n])), nil
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
