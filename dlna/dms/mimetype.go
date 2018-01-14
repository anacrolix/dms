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

// IsMedia returns true for media MIME-types
func (mt mimeType) IsMedia() bool {
	return mt.IsVideo() || mt.IsAudio() || mt.IsImage()
}

// IsVideo returns true for video MIME-types
func (mt mimeType) IsVideo() bool {
	return strings.HasPrefix(string(mt), "video/") || mt == "application/vnd.rn-realmedia-vbr"
}

// IsAudio returns true for audio MIME-types
func (mt mimeType) IsAudio() bool {
	return strings.HasPrefix(string(mt), "audio/")
}

// IsImage returns true for image MIME-types
func (mt mimeType) IsImage() bool {
	return strings.HasPrefix(string(mt), "image/")
}

// Returns the group "type", the part before the '/'.
func (mt mimeType) Type() string {
	return strings.SplitN(string(mt), "/", 2)[0]
}

// Returns the string representation of this MIME-type
func (mt mimeType) String() string {
	return string(mt)
}

// Used to determine the MIME-type for the given path
func MimeTypeByPath(path string) (ret mimeType) {
	defer func() {
		if ret == "video/x-msvideo" {
			ret = "video/avi"
		}
	}()
	ret = mimeTypeByBaseName(path.Base(path))
	if ret != "" {
		return
	}
	ret, _ = mimeTypeByContent(path)
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

func mimeTypeByContent(path string) (ret mimeType, err error) {
	file, err := os.Open(path)
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
	}
}
