## 音频文件解析摩斯码
***
支持从音频文件，包括mp3、wav解析出pcm编码，使用k-means分类，识别音频电信号的长、短音以及字符间隔，并转换为摩斯码。

### 快速使用
引入此包：
```
go get -u github.com/drery/morse
```

使用示例：
```go
package main

import (
	"fmt"
	"github.com/drery/morse"
	"log"
	"os"
)

func main() {
	r, err := os.Open("testdata/morse.mp3")
	if err != nil {
		log.Fatal(err)
	}

	decoder, err := morse.NewDecoder(r, morse.AudioTypeMp3)
	if err != nil {
		log.Fatal(err)
	}

	// 指定morse码在音频中的位置
	pcmBuffer, err := decoder.ParsePart(0, 1.8)
	if err != nil {
		return
	}

	// 解析摩斯码长短音，该示例为：[. - . .]
	dotChars, err := pcmBuffer.DotChars()
	if err != nil {
		return
	}
	fmt.Println(dotChars)

	// 在长短音基础上加上时间间隔，转换为摩斯码文本，该示例为：AI
	morseText := pcmBuffer.MorseText(dotChars)
	fmt.Println(morseText)
}
```

### 实现步骤
- 使用相应的库解析wav/mp3音频文件
- 从音频文件中读取指定时间范围的pcm buffer
- 基于pcm电信号计算平滑功率包络
- 将包络转换为0/1两类信号，0为off信号，1为on信号
- 分别计算on信号与off信号的持续时长
- 使用k-means算法将on信号持续时长分类为：长音(-)与短音(.)
- 将off信号穿插进长短音中，使用[morse_code.go](./morse_code.go)中定义的映射转换为摩斯码文本   

整体实现思路参考了此[python摩斯码解析库](https://github.com/mkouhia/morse-audio-decoder)。