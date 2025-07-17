package morse

import (
	"fmt"
	"github.com/pkg/errors"
	"io"
	"math"
	"sort"
	"strings"
	"sync"
)

const (
	smoothingWindowMs     = 10
	thresholdRatio        = 0.5
	dotDurationMultiplier = 1.5
	wpmGuess              = 20
)

// Decoder ...
type Decoder struct {
	ad audioDecoder
}

// NewDecoder ...
func NewDecoder(r io.ReadSeeker, typ AudioType) (*Decoder, error) {
	gen, ok := audioDecoders[typ]
	if !ok {
		return nil, errors.Errorf("unsupported audio type: %s", typ)
	}

	ad, err := gen(r)
	if err != nil {
		return nil, err
	}

	return &Decoder{ad}, nil
}

// ParsePart ...
func (d Decoder) ParsePart(start, end float64) (*PCMBuffer, error) {
	samples, err := d.ad.PCMBuffer(start, end)
	if err != nil {
		return nil, errors.Wrap(err, "parse audio pcm buffer failed")
	}

	return &PCMBuffer{
		samples:    samples,
		sampleRate: d.ad.SampleRate(),
	}, nil
}

// PCMBuffer ...
type PCMBuffer struct {
	samples      []int
	sampleRate   int
	once         sync.Once
	binarySignal []int
	onSamples    []int
	offSamples   []int
}

// DotChars ...
func (b *PCMBuffer) DotChars() ([]string, error) {
	b.calculate()
	return detectDashesDots(b.onSamples, b.sampleRate)
}

// MorseText ...
func (b *PCMBuffer) MorseText(dotChars []string) string {
	b.calculate()

	charBreakIdxs, wordSpaceIdxs := detectSpaces(b.offSamples)

	return mergeToText(dotChars, charBreakIdxs, wordSpaceIdxs)
}

func (b *PCMBuffer) calculate() {
	b.once.Do(func() {
		// 计算平滑包络
		envelope := smoothedPower(b.samples, b.sampleRate)
		// 二值化信号
		binarySignal := squaredSignal(envelope)
		// 寻找边界
		rising, falling := findEdges(binarySignal)
		onSamples, offSamples := calculateDurations(rising, falling, len(binarySignal))
		b.binarySignal = binarySignal
		b.onSamples = onSamples
		b.offSamples = offSamples
	})
}

// 计算平滑功率包络
func smoothedPower(samples []int, sampleRate int) []float64 {
	windowSize := sampleRate * smoothingWindowMs / 1000
	if windowSize == 0 {
		windowSize = 1
	}

	// 创建汉宁窗
	window := make([]float64, windowSize)
	for i := range window {
		window[i] = 0.5 - 0.5*math.Cos(2*math.Pi*float64(i)/float64(windowSize-1))
	}

	// 归一化
	var sum float64
	for i := range window {
		sum += window[i]
	}
	alpha := 1 / sum
	for i := range window {
		window[i] *= alpha
	}

	// 计算平方信号
	squared := make([]float64, len(samples))
	for i, s := range samples {
		squared[i] = float64(s) * float64(s)
	}

	envelope := convolve(squared, window)

	return envelope
}

func convolve(a, v []float64) []float64 {
	n := len(a)
	m := len(v)

	var result []float64
	if n < m {
		return []float64{}
	}
	resultLen := n - m + 1
	result = make([]float64, resultLen)
	for i := 0; i < resultLen; i++ {
		var sum float64
		for j := 0; j < m; j++ {
			sum += a[i+j] * v[j]
		}
		result[i] = sum
	}

	return result
}

// squaredSignal 将包络转换为二进制信号
func squaredSignal(envelope []float64) []int {
	// 计算阈值
	maxVal := slicesMax(envelope)
	threshold := thresholdRatio * maxVal

	// 二值化
	binary := make([]int, len(envelope))
	for i, v := range envelope {
		if v > threshold {
			binary[i] = 1
		} else {
			binary[i] = 0
		}
	}
	return binary
}

// findEdges 在二进制信号中寻找上升沿和下降沿
func findEdges(binary []int) (rising []int, falling []int) {
	if len(binary) == 0 {
		return
	}

	prev := binary[0]
	for i := 1; i < len(binary); i++ {
		current := binary[i]
		if prev == 0 && current == 1 {
			rising = append(rising, i)
		} else if prev == 1 && current == 0 {
			falling = append(falling, i)
		}
		prev = current
	}
	return
}

// calculateDurations 计算ON和OFF持续时间
func calculateDurations(rising, falling []int, totalLength int) (onSamples, offSamples []int) {
	// 处理开始和结束的特殊情况
	if len(falling) > 0 && len(rising) > 0 {
		if falling[0] < rising[0] {
			rising = append([]int{-1}, rising...)
		}

		if rising[len(rising)-1] > falling[len(falling)-1] {
			falling = append(falling, totalLength-1)
		}
	}

	// 计算ON持续时间
	for i := 0; i < len(falling) && i < len(rising); i++ {
		duration := falling[i] - rising[i]
		onSamples = append(onSamples, duration)
	}

	// 计算OFF持续时间
	for i := 0; i < len(rising)-1 && i < len(falling); i++ {
		duration := rising[i+1] - falling[i]
		offSamples = append(offSamples, duration)
	}

	return
}

// kMeans k-means聚类
func kMeans(data []float64, k int) (centers []float64, labels []int) {
	if len(data) == 0 {
		return
	}
	// 初始化聚类中心
	sortedData := make([]float64, len(data))
	copy(sortedData, data)
	sort.Float64s(sortedData)
	step := len(data) / k
	for i := 0; i < k; i++ {
		idx := min(i*step, len(data)-1)
		centers = append(centers, sortedData[idx])
	}
	// 迭代聚类
	for iter := 0; iter < 100; iter++ {
		labels = make([]int, len(data))
		counts := make([]int, k)
		sums := make([]float64, k)
		// 分配点到最近中心
		for i, d := range data {
			minDist := math.MaxFloat64
			for cIdx, center := range centers {
				dist := math.Abs(d - center)
				if dist < minDist {
					minDist = dist
					labels[i] = cIdx
				}
			}
			counts[labels[i]]++
			sums[labels[i]] += d
		}
		// 更新中心
		newCenters := make([]float64, k)
		for i := 0; i < k; i++ {
			if counts[i] > 0 {
				newCenters[i] = sums[i] / float64(counts[i])
			} else {
				newCenters[i] = centers[i]
			}
		}
		// 检查是否收敛
		changed := false
		for i := range centers {
			if math.Abs(centers[i]-newCenters[i]) > 1e-6 {
				changed = true
				break
			}
		}
		centers = newCenters
		if !changed {
			break
		}
	}
	// 按中心值排序
	sortedCenters := make([]float64, len(centers))
	copy(sortedCenters, centers)
	sort.Float64s(sortedCenters)
	remapping := make(map[int]int)
	for origIdx, c := range centers {
		for sortedIdx, sc := range sortedCenters {
			if c == sc {
				remapping[origIdx] = sortedIdx
				break
			}
		}
	}
	for i, label := range labels {
		labels[i] = remapping[label]
	}

	return centers, labels
}

// 检测短音和长音
func detectDashesDots(onSamples []int, sampleRate int) ([]string, error) {
	if len(onSamples) == 0 {
		return nil, nil
	}

	// 转换为浮点数用于聚类
	floatSamples := make([]float64, len(onSamples))
	for i, s := range onSamples {
		floatSamples[i] = float64(s)
	}
	k := min(2, len(onSamples))
	centers, labels := kMeans(floatSamples, k)
	if len(centers) == 0 {
		return nil, fmt.Errorf("聚类失败")
	}

	// 确定点划标签
	sortedCenters := make([]float64, len(centers))
	copy(sortedCenters, centers)
	sort.Float64s(sortedCenters)

	dotLabel := -1
	dashLabel := -1
	if k == 1 {
		dotDuration := float64(sampleRate) / (float64(wpmGuess) * 60 / 1000)
		isDot := sortedCenters[0] < dotDuration*dotDurationMultiplier

		if isDot {
			dotLabel = 0
		} else {
			dashLabel = 0
		}
	} else {
		for label, center := range centers {
			if center == sortedCenters[0] {
				dotLabel = label
			} else if center == sortedCenters[1] {
				dashLabel = label
			}
		}
	}

	// 生成点划序列
	result := make([]string, len(onSamples))
	for i, label := range labels {
		switch {
		case dotLabel != -1 && label == dotLabel:
			result[i] = "."
		case dashLabel != -1 && label == dashLabel:
			result[i] = "-"
		default:
			result[i] = "?"
		}
	}

	return result, nil
}

// 检测不同类型间隔
func detectSpaces(offSamples []int) (charBreakIdxs, wordSpaceIdxs []int) {
	if len(offSamples) == 0 {
		return
	}

	// 转换为浮点数用于聚类
	floatSamples := make([]float64, len(offSamples))
	for i, s := range offSamples {
		floatSamples[i] = float64(s)
	}

	k := min(3, len(offSamples))
	_, labels := kMeans(floatSamples, k)

	if len(labels) == 0 {
		return
	}

	// 获取每个标签的平均时长（以确定类型）
	means := make(map[int]float64)
	counts := make(map[int]int)
	sums := make(map[int]float64)

	for i, lbl := range labels {
		sums[lbl] += floatSamples[i]
		counts[lbl]++
	}

	for lbl := range sums {
		if counts[lbl] > 0 {
			means[lbl] = sums[lbl] / float64(counts[lbl])
		}
	}

	// 确定最短、中等和最长的间隔
	type pair struct {
		label int
		mean  float64
	}
	pairs := make([]pair, 0, len(means))
	for lbl, mean := range means {
		pairs = append(pairs, pair{label: lbl, mean: mean})
	}

	sort.Slice(pairs, func(i, j int) bool {
		return pairs[i].mean < pairs[j].mean
	})

	var intraSpaceLabel, charSpaceLabel, wordSpaceLabel int
	for i, p := range pairs {
		if i == 0 {
			intraSpaceLabel = p.label
		} else if i == 1 {
			charSpaceLabel = p.label
		} else if i == 2 {
			wordSpaceLabel = p.label
		}
	}
	_ = charSpaceLabel

	// 字符分割点
	for i, lbl := range labels {
		if lbl != intraSpaceLabel {
			charBreakIdxs = append(charBreakIdxs, i+1)
		}
	}

	// 单词分割点
	for i, lbl := range labels {
		if len(pairs) == 3 && lbl == wordSpaceLabel {
			wordSpaceIdxs = append(wordSpaceIdxs, i+1)
		}
	}

	return
}

// 将点划序列合并为文本
func mergeToText(dashDotChars []string, charBreakIdxs, wordSpaceIdxs []int) string {
	charStartIdx := 0
	charEndIdx := 0
	var morseCharacters []string

	// 生成每个字符的摩斯码
	for _, breakIdx := range charBreakIdxs {
		charEndIdx = min(breakIdx, len(dashDotChars))
		chars := dashDotChars[charStartIdx:charEndIdx]
		morseCharacters = append(morseCharacters, strings.Join(chars, ""))
		charStartIdx = charEndIdx
	}
	if charStartIdx < len(dashDotChars) {
		morseCharacters = append(morseCharacters, strings.Join(dashDotChars[charStartIdx:], ""))
	}

	// 添加单词分隔符
	textSegments := []string{}
	wordStart := 0
	for _, spaceIdx := range wordSpaceIdxs {
		if spaceIdx < len(morseCharacters) {
			word := morseCharacters[wordStart:spaceIdx]
			textSegments = append(textSegments, translateMorseWord(word, morseMap))
			wordStart = spaceIdx + 1
		}
	}
	if wordStart < len(morseCharacters) {
		word := morseCharacters[wordStart:]
		textSegments = append(textSegments, translateMorseWord(word, morseMap))
	}

	// 添加可能的遗漏字符
	if len(morseCharacters) > 0 && len(textSegments) == 0 {
		textSegments = append(textSegments, translateMorseWord(morseCharacters, morseMap))
	}

	return strings.Join(textSegments, " ")
}

// 转换摩斯单词
func translateMorseWord(morseChars []string, morseMap map[string]string) string {
	var word strings.Builder
	for _, morse := range morseChars {
		char := morseMap[morse]
		if char == "" {
			char = "?"
		}
		word.WriteString(char)
	}
	return word.String()
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func slicesMax(s []float64) float64 {
	if len(s) == 0 {
		return 0
	}
	maxVal := s[0]
	for _, v := range s[1:] {
		if v > maxVal {
			maxVal = v
		}
	}
	return maxVal
}
