package filter

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"

	"github.com/duo/octopus/internal/common"
	"github.com/wdvxdr1123/go-silk"

	log "github.com/sirupsen/logrus"
)

const sampleRate = 24000

type SilkFilter struct {
}

// Telegram -> QQ/WeChat: convert opus voice to silk
// QQ/WeChat -> Telegram: convert silk voice to opus
func (f SilkFilter) Process(in *common.OctopusEvent) *common.OctopusEvent {
	if in.Vendor.Type == "qq" || in.Vendor.Type == "wechat" {
		if in.Type == common.EventAudio {
			blob := in.Data.(*common.BlobData)
			if blob.Mime == "audio/ogg" { // from Telegram
				if in.Vendor.Type == "qq" {
					if data, err := ogg2silk(blob.Binary); err != nil {
						log.Warnf("Failed to convert ogg to silk: %v", err)
					} else {
						blob.Mime = "audio/silk"
						blob.Binary = data
					}
				} else {
					if data, err := ogg2mp3(blob.Binary); err != nil {
						log.Warnf("Failed to convert ogg to mp3: %v", err)
					} else {
						in.Type = common.EventFile
						blob.Mime = "audio/mpeg"
						blob.Binary = data

						randBytes := make([]byte, 4)
						rand.Read(randBytes)
						blob.Name = fmt.Sprintf("VOICE_%s.mp3", hex.EncodeToString(randBytes))
					}
				}
			} else {
				// from QQ/WeChat
				if data, err := silk2ogg(blob.Binary); err != nil {
					log.Warnf("Failed to convert silk to ogg: %v", err)
				} else {
					blob.Mime = "audio/ogg"
					blob.Binary = data
				}
			}
		}
	}
	return in
}

func silk2ogg(rawData []byte) ([]byte, error) {
	pcmData, err := silk.DecodeSilkBuffToPcm(rawData, sampleRate)
	if err != nil {
		return nil, err
	}

	pcmFile, err := os.CreateTemp("", "pcm-")
	if err != nil {
		return nil, err
	}
	defer os.Remove(pcmFile.Name())
	os.WriteFile(pcmFile.Name(), pcmData, 0o644)

	wavFile, err := os.CreateTemp("", "wav-")
	if err != nil {
		return nil, err
	}
	defer os.Remove(wavFile.Name())
	{
		cmd := exec.Command(
			"ffmpeg", "-f", "s16le", "-ar", "24000", "-ac", "1", "-y", "-i", pcmFile.Name(), "-f", "wav", "-af", "volume=7.812500", wavFile.Name())
		if err := cmd.Start(); err != nil {
			return nil, err
		}
		if err := cmd.Wait(); err != nil {
			return nil, err
		}
	}

	oggFile, err := os.CreateTemp("", "ogg-")
	if err != nil {
		return nil, err
	}
	defer os.Remove(oggFile.Name())
	{
		cmd := exec.Command(
			"ffmpeg", "-y", "-i", wavFile.Name(), "-c:a", "libopus", "-b:a", "24K", "-f", "ogg", oggFile.Name())
		if err := cmd.Start(); err != nil {
			return nil, err
		}

		if err := cmd.Wait(); err != nil {
			return nil, err
		}
	}

	return os.ReadFile(oggFile.Name())
}

func ogg2silk(rawData []byte) ([]byte, error) {
	oggFile, err := os.CreateTemp("", "ogg-")
	if err != nil {
		return nil, err
	}
	defer os.Remove(oggFile.Name())
	os.WriteFile(oggFile.Name(), rawData, 0o644)

	wavFile, err := os.CreateTemp("", "wav-")
	if err != nil {
		return nil, err
	}
	defer os.Remove(wavFile.Name())
	{
		cmd := exec.Command(
			"ffmpeg", "-y", "-i", oggFile.Name(), "-f", "s16le", "-ar", "24000", "-ac", "1", wavFile.Name())
		if err := cmd.Start(); err != nil {
			return nil, err
		}
		if err := cmd.Wait(); err != nil {
			return nil, err
		}
	}

	wavData, err := os.ReadFile(wavFile.Name())
	if err != nil {
		return nil, err
	}

	silkData, err := silk.EncodePcmBuffToSilk(wavData, sampleRate, sampleRate, true)
	if err != nil {
		return nil, err
	}

	return silkData, nil
}

func ogg2mp3(rawData []byte) ([]byte, error) {
	oggFile, err := os.CreateTemp("", "ogg-")
	if err != nil {
		return nil, err
	}
	defer os.Remove(oggFile.Name())
	os.WriteFile(oggFile.Name(), rawData, 0o644)

	mp3File, err := os.CreateTemp("", "mp3-")
	if err != nil {
		return nil, err
	}
	defer os.Remove(mp3File.Name())
	{
		cmd := exec.Command("ffmpeg", "-y", "-i", oggFile.Name(), "-f", "mp3", mp3File.Name())
		if err := cmd.Start(); err != nil {
			return nil, err
		}
		if err := cmd.Wait(); err != nil {
			return nil, err
		}
	}

	return os.ReadFile(mp3File.Name())
}
