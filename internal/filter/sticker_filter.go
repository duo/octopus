package filter

import (
	"bytes"
	"os"
	"os/exec"

	"github.com/duo/octopus/internal/common"

	"github.com/Benau/tgsconverter/libtgsconverter"
	"github.com/gabriel-vasile/mimetype"
	"github.com/tidwall/gjson"

	log "github.com/sirupsen/logrus"
)

type StickerFilter struct {
	FromMaster bool
}

func (f StickerFilter) Process(in *common.OctopusEvent) *common.OctopusEvent {
	if f.FromMaster {
		return f.processM2S(in)
	}
	return f.processS2M(in)
}

// Telegram -> QQ/WeChat: convert webm and tgs image to gif
func (f StickerFilter) processM2S(in *common.OctopusEvent) *common.OctopusEvent {
	if in.Vendor.Type == "qq" || in.Vendor.Type == "wechat" {
		if in.Type == common.EventPhoto || in.Type == common.EventSticker {
			var blob *common.BlobData
			if in.Type == common.EventPhoto {
				blob = in.Data.([]*common.BlobData)[0]
			} else {
				blob = in.Data.(*common.BlobData)
			}
			switch blob.Mime {
			case "video/webm":
				if data, err := webm2gif(blob.Binary); err != nil {
					log.Warnf("Failed to convert webm to gif: %v", err)
				} else {
					blob.Mime = "image/gif"
					blob.Name = blob.Name + ".gif"
					blob.Binary = data
				}
			case "video/mp4":
				// TODO: solve export gif over size
				if in.Vendor.Type == "qq" {
					in.Type = common.EventVideo
					in.Data = blob
				}
			case "application/gzip": // TGS
				if data, err := tgs2gif(blob.Binary); err != nil {
					log.Warnf("Failed to convert tgs to gif: %v", err)
				} else {
					blob.Mime = "image/gif"
					blob.Name = blob.Name + ".gif"
					blob.Binary = data
				}
			}
		}

	}
	return in
}

// QQ/WeChat -> Telegram
func (f StickerFilter) processS2M(in *common.OctopusEvent) *common.OctopusEvent {
	if in.Vendor.Type == "qq" || in.Vendor.Type == "wechat" {
		if in.Type == common.EventSticker {
			blob := in.Data.(*common.BlobData)
			mime := mimetype.Detect(blob.Binary)
			blob.Mime = mime.String()
			if blob.Mime == "image/jpeg" {
				if data, err := jpeg2webp(blob.Binary); err != nil {
					log.Warnf("Failed to convert jpeg to webp: %v", err)
				} else {
					blob.Mime = "image/webp"
					blob.Binary = data
				}
			} else if blob.Mime == "image/gif" {
				if probe, err := ffprobe(blob.Binary); err == nil {
					if gjson.Get(probe, "streams.0.nb_frames").Int() == 1 {
						blob.Mime = "image/png"
					}
				} else {
					log.Warnf("Failed to probe gif: %v", err)
				}
			}
		}
	}
	return in
}

func ffprobe(rawData []byte) (string, error) {
	probeFile, err := os.CreateTemp("", "probe-")
	if err != nil {
		return "", err
	}
	defer os.Remove(probeFile.Name())
	os.WriteFile(probeFile.Name(), rawData, 0o644)

	var out bytes.Buffer
	cmd := exec.Command("ffprobe", probeFile.Name(), "-show_format", "-show_streams", "-of", "json")
	cmd.Stdout = &out
	if err := cmd.Start(); err != nil {
		return "", err
	}
	if err := cmd.Wait(); err != nil {
		return "", err
	}

	return out.String(), nil
}

func webm2gif(rawData []byte) ([]byte, error) {
	webmFile, err := os.CreateTemp("", "webm-")
	if err != nil {
		return nil, err
	}
	defer os.Remove(webmFile.Name())
	os.WriteFile(webmFile.Name(), rawData, 0o644)

	gifFile, err := os.CreateTemp("", "gif-")
	if err != nil {
		return nil, err
	}
	defer os.Remove(gifFile.Name())
	{
		cmd := exec.Command("ffmpeg", "-y", "-i", webmFile.Name(), "-f", "gif", gifFile.Name())
		if err := cmd.Start(); err != nil {
			return nil, err
		}
		if err := cmd.Wait(); err != nil {
			return nil, err
		}
	}

	return os.ReadFile(gifFile.Name())
}

func tgs2gif(rawData []byte) ([]byte, error) {
	opt := libtgsconverter.NewConverterOptions()
	opt.SetExtension("gif")
	opt.SetScale(0.5)

	ret, err := libtgsconverter.ImportFromData(rawData, opt)
	if err != nil {
		return nil, err
	}

	return ret, nil
}

func jpeg2webp(rawData []byte) ([]byte, error) {
	jpegFile, err := os.CreateTemp("", "jpg-")
	if err != nil {
		return nil, err
	}
	defer os.Remove(jpegFile.Name())
	os.WriteFile(jpegFile.Name(), rawData, 0o644)

	webpFile, err := os.CreateTemp("", "webp-")
	if err != nil {
		return nil, err
	}
	defer os.Remove(webpFile.Name())
	{
		cmd := exec.Command("ffmpeg", "-y", "-i", jpegFile.Name(), "-c:v", "libwebp", "-lossless", "0", "-f", "webp", webpFile.Name())
		if err := cmd.Start(); err != nil {
			return nil, err
		}
		if err := cmd.Wait(); err != nil {
			return nil, err
		}
	}

	return os.ReadFile(webpFile.Name())
}
