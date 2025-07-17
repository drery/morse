package morse

import "io"

func init() {
	registerAudioDecoder(AudioTypeMp3, newMP3Decoder)
	registerAudioDecoder(AudioTypeWav, newWavDecoder)
}

type audioDecoder interface {
	PCMBuffer(start, end float64) ([]int, error)
	SampleRate() int
}

type audioDecoderGenerator func(r io.ReadSeeker) (audioDecoder, error)

var audioDecoders map[AudioType]audioDecoderGenerator

// AudioType ...
type AudioType string

// AudioTypeWav ...
const (
	AudioTypeWav AudioType = "wav"
	AudioTypeMp3 AudioType = "mp3"
)

func registerAudioDecoder(a AudioType, g audioDecoderGenerator) {
	if audioDecoders == nil {
		audioDecoders = make(map[AudioType]audioDecoderGenerator)
	}

	audioDecoders[a] = g
}

