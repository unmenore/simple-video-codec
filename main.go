package main

import (
	"flag"
	"io"
	"log"
	"os"
)

func main() {
	var width, height int
	flag.IntVar(&width, "width", 640, "width of the video")
	flag.IntVar(&height, "height", 480, "height of the video")
	flag.Parse()

	frames := make([][]byte, 0)

	for {

		//Считываем видео в формате rgb24, в котором каждый пиксель равень одному байту
		//следовательно общий размер кадра равен width*height*3
		frame := make([]byte, width*height*3)
		//Считываем кадр из stdin
		if _, err := io.ReadFull(os.Stdin, frame); err != nil {
			break
		}

		frames = append(frames, frame)
	}

	//Тут теперь у нас есть необработанное видео в виде массива байтов

	rawSize := size(frames)
	log.Printf("Raw video size: %d bytes", rawSize)

	for i, frame := range frames {

		//Сначала преобразуем каждый кдр в формате yuv420
		//Пояснение расположения пикселей в кадре находится в фале rgb.md

		Y := make([]byte, width*height)
		U := make([]float64, width*height)
		V := make([]float64, width*height)

		for j := 0; j < width*height; j++ {
			//Конвертируем пикслеи из RGB в YUV
			r, g, b := float64(frame[3*j]), float64(frame[3*j+1]), float64(frame[3*j+2])

			//Данные коэффициенты взяты с https://en.wikipedia.org/wiki/YUV#Y%E2%80%B2UV444_to_RGB888_conversion
			//На практике коэффициенты варируются в зависимости от стандарта
			y := +0.299*r + 0.587*g + 0.114*b
			u := -0.169*r - 0.331*g + 0.449*b + 128
			v := 0.499*r - 0.418*g - 0.0813*b + 128

			Y[j] = uint8(y)
			U[j] = u
			V[j] = v
		}

		//Теперь уменьшаем выборку компонентов из V и U
		//В процессе мы берем 4 пикселя и усредняем их вместе

		vDownslamped := make([]byte, width*height/4)
		uDownslamped := make([]byte, width*height/4)

		for x := 0; x < height; x += 2 {
			for y := 0; y < width; y += 2 {
				//Мы усредним U и V компоненты 4 пикселей, которые разделяют этот U и V компонент

				u := (U[x*width*y] + U[x*width*(y+1)] + U[(x+1)*width*y] + U[(x+1)*width*(y+1)]) / 4
				v := (V[x*width*y] + V[x*width*(y+1)] + V[(x+1)*width*y] + V[(x+1)*width*(y+1)]) / 4

				//Сохраняем уменьшенные компоненты U и V в байтовых фрагментах

				vDownslamped[x/2*width/2+y/2] = uint8(v)
				uDownslamped[x/2*width/2+y/2] = uint8(u)
			}
		}

		yuvFrame := make([]byte, len(Y)+len(uDownslamped)+len(vDownslamped))

		// Теперь нам нужно сохранить значения YUV в байтовом фрагменте. Чтобы сделать данные более
		// сжимаемыми, мы сначала сохраним все значения Y, затем все значения U,
		// затем все значения V. Это называется плоским форматом.
		//
		// Cоседние значения Y, U и V с большей вероятностью будут
		// похожи, чем сами Y, U и V. Следовательно, сохранение компонентов
		// в плоском формате позволит сохранить больше данных позже.

		copy(yuvFrame, Y)
		copy(yuvFrame[len(Y):], uDownslamped)
		copy(yuvFrame[len(Y)+len(uDownslamped):], vDownslamped)

		frames[i] = yuvFrame
	}

	//Теперь имеется видео в yuv кодировке с половиной занимаемого пространства

}

func size(frames [][]byte) int {
	var size int
	for _, frame := range frames {
		size += len(frame)
	}
	return size

}

func clamp(x, min, max float64) float64 {
	if x < min {
		return min
	}

	if x > max {
		return max
	}

	return x

}
