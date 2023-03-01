package filter

import (
	"os"
	"os/exec"

	"github.com/duo/octopus/internal/common"

	"github.com/Benau/tgsconverter/libtgsconverter"

	log "github.com/sirupsen/logrus"
)

type StickerFilter struct {
}

// Telegram -> QQ/WeChat: convert webm and tgs image to gif
func (f StickerFilter) Process(in *common.OctopusEvent) *common.OctopusEvent {
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
