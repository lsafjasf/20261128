// Package classify maps file extensions to archive classes.
// It only depends on the Go standard library.
package classify

import (
	"mime"
	"path/filepath"
	"strings"
)

const (
	ClassMedia   = "media"
	ClassSidecar = "sidecar"
	ClassOther   = "other"
)

// mediaExts maps a lower-cased extension (with leading dot) to a media kind.
var mediaExts = map[string]string{
	// images, including camera raw formats
	".jpg": "image", ".jpeg": "image", ".png": "image", ".gif": "image",
	".bmp": "image", ".tif": "image", ".tiff": "image", ".webp": "image",
	".heic": "image", ".heif": "image", ".dng": "image", ".cr2": "image",
	".cr3": "image", ".nef": "image", ".arw": "image", ".rw2": "image",
	".orf": "image", ".raf": "image", ".pef": "image",
	// video
	".mp4": "video", ".mov": "video", ".mkv": "video", ".avi": "video",
	".mxf": "video", ".mts": "video", ".m2ts": "video", ".wmv": "video",
	".flv": "video", ".webm": "video",
	// audio
	".wav": "audio", ".mp3": "audio", ".flac": "audio", ".aac": "audio",
	".m4a": "audio", ".ogg": "audio", ".aiff": "audio",
}

// sidecarExts lists metadata / companion file extensions.
var sidecarExts = map[string]bool{
	".xmp":  true,
	".json": true,
	".xml":  true,
	".srt":  true,
	".vtt":  true,
	".ass":  true,
	".ssa":  true,
	".lrv":  true, // low-resolution proxy
	".thm":  true, // thumbnail companion
	".aae":  true, // Apple edit metadata
}

func init() {
	// Types missing from the built-in MIME table or needing archive semantics.
	_ = mime.AddExtensionType(".cr3", "image/x-canon-cr3")
	_ = mime.AddExtensionType(".cr2", "image/x-canon-cr2")
	_ = mime.AddExtensionType(".nef", "image/x-nikon-nef")
	_ = mime.AddExtensionType(".arw", "image/x-sony-arw")
	_ = mime.AddExtensionType(".dng", "image/x-adobe-dng")
	_ = mime.AddExtensionType(".xmp", "application/rdf+xml")
	_ = mime.AddExtensionType(".lrv", "video/mp4")
	_ = mime.AddExtensionType(".aae", "application/xml")
}

// Ext returns the normalized lower-cased extension of name.
func Ext(name string) string {
	return strings.ToLower(filepath.Ext(name))
}

// IsMedia reports whether ext belongs to a known media format.
func IsMedia(ext string) bool {
	_, ok := mediaExts[ext]
	return ok
}

// IsSidecar reports whether ext is a known sidecar / companion format.
func IsSidecar(ext string) bool {
	return sidecarExts[ext]
}

// Class returns media, sidecar or other for the given file name.
func Class(name string) string {
	ext := Ext(name)
	switch {
	case IsMedia(ext):
		return ClassMedia
	case IsSidecar(ext):
		return ClassSidecar
	default:
		return ClassOther
	}
}

// MediaKind returns image, video or audio for media extensions, else "".
func MediaKind(name string) string {
	return mediaExts[Ext(name)]
}

// MIME returns a MIME type based on the extension, falling back to the
// generic application/octet-stream. It never inspects content.
func MIME(name string) string {
	if t := mime.TypeByExtension(Ext(name)); t != "" {
		return t
	}
	return "application/octet-stream"
}

// Stem returns the file name without its final extension.
func Stem(name string) string {
	return strings.TrimSuffix(name, filepath.Ext(name))
}
