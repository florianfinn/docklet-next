// Source: getarcaneapp/kit/pkg/image.go
// Revision: 9c61043fc52d1977d644f075d9c168335c3a6df7
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: None

package kit

func GetImageMimeType(ext string) string {
	switch ext {
	case "jpg", "jpeg":
		return "image/jpeg"
	case "png":
		return "image/png"
	case "svg":
		return "image/svg+xml"
	case "ico":
		return "image/x-icon"
	case "gif":
		return "image/gif"
	case "webp":
		return "image/webp"
	case "avif":
		return "image/avif"
	case "heic":
		return "image/heic"
	default:
		return ""
	}
}
