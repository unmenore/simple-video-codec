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
		}
	}

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
