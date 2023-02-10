package filter

import (
	"bytes"
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
	"os/exec"

	"github.com/duo/octopus/internal/common"
	"github.com/wdvxdr1123/go-silk"

	log "github.com/sirupsen/logrus"
)

const sampleRate = 24000
const numChannels = 1
const precision = 2

type waveHeader struct {
	RiffMark      [4]byte
	FileSize      int32
	WaveMark      [4]byte
	FmtMark       [4]byte
	FormatSize    int32
	FormatType    int16
	NumChans      int16
	SampleRate    int32
	ByteRate      int32
	BytesPerFrame int16
	BitsPerSample int16
	DataMark      [4]byte
	DataSize      int32
}

type SilkFilter struct {
}

// Telegram -> QQ/WeChat: convert opus voice to silk
// QQ/WeChat -> Telegram: convert silk voice to opus
func (f SilkFilter) Process(in *common.OctopusEvent) (*common.OctopusEvent, bool) {
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
	return in, true
}

func silk2ogg(rawData []byte) ([]byte, error) {
	pcmData, err := silk.DecodeSilkBuffToPcm(rawData, sampleRate)
	if err != nil {
		return nil, err
	}

	header := waveHeader{
		RiffMark:      [4]byte{'R', 'I', 'F', 'F'},
		FileSize:      int32(44 + len(pcmData)),
		WaveMark:      [4]byte{'W', 'A', 'V', 'E'},
		FmtMark:       [4]byte{'f', 'm', 't', ' '},
		FormatSize:    16,
		FormatType:    1,
		NumChans:      int16(numChannels),
		SampleRate:    int32(sampleRate),
		ByteRate:      int32(sampleRate * numChannels * precision),
		BytesPerFrame: int16(numChannels * precision),
		BitsPerSample: int16(precision) * 8,
		DataMark:      [4]byte{'d', 'a', 't', 'a'},
		DataSize:      int32(len(pcmData)),
	}

	buf := &bytes.Buffer{}
	if err := binary.Write(buf, binary.LittleEndian, &header); err != nil {
		return nil, err
	}
	if _, err := buf.Write(pcmData); err != nil {
		return nil, err
	}

	cmd := exec.Command(
		"ffmpeg", "-i", "pipe:0", "-c:a", "libopus", "-b:a", "24K", "-f", "ogg", "pipe:1")
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

func ogg2silk(rawData []byte) ([]byte, error) {
	buf := bytes.NewBuffer(rawData)

	cmd := exec.Command("ffmpeg", "-i", "pipe:0", "-f", "s16le", "-ar", "24000", "-ac", "1", "pipe:1")
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

	waveBuf := &bytes.Buffer{}
	io.Copy(waveBuf, stdout)

	if err := cmd.Wait(); err != nil {
		return nil, err
	}

	silkData, err := silk.EncodePcmBuffToSilk(waveBuf.Bytes(), sampleRate, sampleRate, true)
	if err != nil {
		return nil, err
	}

	return silkData, nil
}

func ogg2mp3(rawData []byte) ([]byte, error) {
	buf := bytes.NewBuffer(rawData)

	cmd := exec.Command(
		"ffmpeg", "-i", "pipe:0", "-f", "mp3", "pipe:1")
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
