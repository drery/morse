package morse

import (
	"github.com/go-audio/wav"
	"github.com/pkg/errors"
	"io"
)

type wavDecoder struct {
	decoder *wav.Decoder
}

func newWavDecoder(r io.ReadSeeker) (audioDecoder, error) {
	decoder := wav.NewDecoder(r)
	if !decoder.IsValidFile() {
		return nil, errors.Errorf("invalid wav file")
	}

	return &wavDecoder{decoder: decoder}, nil
}

// SampleRate Decode ...
func (wd wavDecoder) SampleRate() int {
	return int(wd.decoder.SampleRate)
}

// PCMBuffer ...
func (wd wavDecoder) PCMBuffer(start, end float64) ([]int, error) {
	decoder := wd.decoder
	buf, err := decoder.FullPCMBuffer()
	if err != nil {
		return nil, err
	}

	sampleRate := decoder.SampleRate
	numChannels := decoder.NumChans
	samplesToRead := int(float64(sampleRate)*end) * int(numChannels)
	pcmData := buf.Data
	if samplesToRead < len(buf.Data) {
		pcmData = buf.Data[:samplesToRead]
	}

	return pcmData, nil
}
