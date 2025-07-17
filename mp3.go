package morse

import (
	"github.com/go-audio/audio"
	"github.com/hajimehoshi/go-mp3"
	"github.com/pkg/errors"
	"io"
)

type mp3Decoder struct {
	decoder *mp3.Decoder
}

func newMP3Decoder(r io.ReadSeeker) (audioDecoder, error) {
	decoder, err := mp3.NewDecoder(r)
	if err != nil {
		return nil, errors.Wrap(err, "new mp3 decoder failed")
	}

	return &mp3Decoder{decoder: decoder}, nil
}

// SampleRate Decode ...
func (md mp3Decoder) SampleRate() int {
	return md.decoder.SampleRate()
}

// PCMBuffer ...
func (md mp3Decoder) PCMBuffer(start, end float64) ([]int, error) {
	sampleRate := md.decoder.SampleRate()
	samplesNeeded := int(end*float64(sampleRate)) * 2
	pcmBuffer := &audio.IntBuffer{
		Format: &audio.Format{
			NumChannels: 1, // go-mp3 始终输出单声道
			SampleRate:  sampleRate,
		},
		Data: make([]int, 0, samplesNeeded),
	}
	targetSamples := samplesNeeded
	readBuffer := make([]byte, 2048)

	for targetSamples > 0 {
		toRead := targetSamples * 2
		if toRead > len(readBuffer) {
			toRead = len(readBuffer)
		}
		if toRead%2 != 0 {
			toRead-- // 确保读取偶数字节
		}

		n, err := md.decoder.Read(readBuffer[:toRead])
		if err != nil && err != io.EOF {
			return nil, errors.Wrap(err, "read mp3 failed")
		}
		if err == io.EOF {
			break
		}

		for i := 0; i < n; i += 2 {
			sample := int(int16(readBuffer[i]) | int16(readBuffer[i+1])<<8)
			pcmBuffer.Data = append(pcmBuffer.Data, sample)
			targetSamples--

			if targetSamples <= 0 {
				break
			}
		}
	}

	return pcmBuffer.Data, nil
}
