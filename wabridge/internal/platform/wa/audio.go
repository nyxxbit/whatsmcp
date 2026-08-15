package wa

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"math"
	"math/rand"
)

// analyzeOggOpus extrai a duração e gera um waveform de um Ogg Opus (para a nota
// de voz aparecer com a "ondinha" no WhatsApp). Porta a lógica do bridge legado.
func analyzeOggOpus(data []byte) (duration uint32, waveform []byte, err error) {
	if len(data) < 4 || string(data[0:4]) != "OggS" {
		return 0, nil, fmt.Errorf("não é um Ogg válido (sem assinatura OggS)")
	}

	var (
		lastGranule   uint64
		sampleRate    uint32 = 48000
		preSkip       uint16
		foundOpusHead bool
	)

	for i := 0; i < len(data); {
		if i+27 >= len(data) {
			break
		}
		if string(data[i:i+4]) != "OggS" {
			i++
			continue
		}
		granulePos := binary.LittleEndian.Uint64(data[i+6 : i+14])
		pageSeqNum := binary.LittleEndian.Uint32(data[i+18 : i+22])
		numSegments := int(data[i+26])
		if i+27+numSegments >= len(data) {
			break
		}
		segmentTable := data[i+27 : i+27+numSegments]
		pageSize := 27 + numSegments
		for _, segLen := range segmentTable {
			pageSize += int(segLen)
		}
		if !foundOpusHead && pageSeqNum <= 1 {
			pageData := data[i : i+pageSize]
			if headPos := bytes.Index(pageData, []byte("OpusHead")); headPos >= 0 && headPos+12 < len(pageData) {
				headPos += 8 // pula o marcador "OpusHead"
				if headPos+12 <= len(pageData) {
					preSkip = binary.LittleEndian.Uint16(pageData[headPos+10 : headPos+12])
					sampleRate = binary.LittleEndian.Uint32(pageData[headPos+12 : headPos+16])
					foundOpusHead = true
				}
			}
		}
		if granulePos != 0 {
			lastGranule = granulePos
		}
		i += pageSize
	}

	if lastGranule > 0 {
		seconds := float64(lastGranule-uint64(preSkip)) / float64(sampleRate)
		duration = uint32(math.Ceil(seconds))
	} else {
		duration = uint32(float64(len(data)) / 2000.0) // estimativa grosseira
	}
	if duration < 1 {
		duration = 1
	} else if duration > 300 {
		duration = 300
	}
	return duration, placeholderWaveform(duration), nil
}

// placeholderWaveform gera um waveform sintético de 64 bytes, determinístico por
// duração (mesma duração → mesmo desenho), com aparência natural.
func placeholderWaveform(duration uint32) []byte {
	const waveformLength = 64
	waveform := make([]byte, waveformLength)
	rng := rand.New(rand.NewSource(int64(duration)))

	baseAmplitude := 35.0
	frequencyFactor := float64(min(int(duration), 120)) / 30.0

	for i := range waveform {
		pos := float64(i) / float64(waveformLength)
		val := baseAmplitude * math.Sin(pos*math.Pi*frequencyFactor*8)
		val += (baseAmplitude / 2) * math.Sin(pos*math.Pi*frequencyFactor*16)
		val += (rng.Float64() - 0.5) * 15
		val *= 0.7 + 0.3*math.Sin(pos*math.Pi) // fade-in/out
		val += 50                              // baseline de voz
		if val < 0 {
			val = 0
		} else if val > 100 {
			val = 100
		}
		waveform[i] = byte(val)
	}
	return waveform
}
