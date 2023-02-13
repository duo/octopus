package filter

import (
	"bytes"
	"io"
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
		if in.Type == common.EventPhoto {
			photos := in.Data.([]*common.BlobData)
			if len(photos) == 1 {
				blob := photos[0]
				switch blob.Mime {
				case "video/webm":
					if data, err := webm2gif(blob.Binary); err != nil {
						log.Warnf("Failed to convert webm to gif: %v", err)
					} else {
						blob.Mime = "image/gif"
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
						blob.Binary = data
					}
				}
			}
		}

	}
	return in
}

func webm2gif(rawData []byte) ([]byte, error) {
	buf := bytes.NewBuffer(rawData)

	cmd := exec.Command(
		"ffmpeg", "-i", "pipe:0", "-f", "gif", "pipe:1")
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}

	if err := cmd.Start(); err != nil {
		return nil, err
	}

	io.Copy(stdin, buf)
	stdin.Close()

	outputBuf := &bytes.Buffer{}
	io.Copy(outputBuf, stdout)

	if err := cmd.Wait(); err != nil {
		return nil, err
	}

	return outputBuf.Bytes(), nil
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
