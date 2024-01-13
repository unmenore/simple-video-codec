package main

import (
	"bytes"
	"compress/flate"
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

		uDownsampled := make([]byte, width*height/4)
		vDownsampled := make([]byte, width*height/4)
		for x := 0; x < height; x += 2 {
			for y := 0; y < width; y += 2 {

				u := (U[x*width+y] + U[x*width+y+1] + U[(x+1)*width+y] + U[(x+1)*width+y+1]) / 4
				v := (V[x*width+y] + V[x*width+y+1] + V[(x+1)*width+y] + V[(x+1)*width+y+1]) / 4

				uDownsampled[x/2*width/2+y/2] = uint8(u)
				vDownsampled[x/2*width/2+y/2] = uint8(v)
			}
		}

		yuvFrame := make([]byte, len(Y)+len(uDownsampled)+len(vDownsampled))

		// Теперь нам нужно сохранить значения YUV в байтовом фрагменте. Чтобы сделать данные более
		// сжимаемыми, мы сначала сохраним все значения Y, затем все значения U,
		// затем все значения V. Это называется плоским форматом.
		//
		// .. Соседние значения Y, U и V с большей вероятностью будут
		// похожи, чем сами Y, U и V. Следовательно, сохранение компонентов
		// в плоском формате позволит сохранить больше данных позже.

		copy(yuvFrame, Y)
		copy(yuvFrame[len(Y):], uDownsampled)
		copy(yuvFrame[len(Y)+len(uDownsampled):], vDownsampled)

		frames[i] = yuvFrame
	}

	//Теперь имеется видео в yuv кодировке с половиной занимаемого пространства

	yuvSize := size(frames)
	log.Printf("YUV420P size: %d bytes (%0.2f%% original size)", yuvSize, 100*float32(yuvSize)/float32(rawSize))

	//Также можем записать это в файл который можно воспроизвести с помощью ffplay
	//ffplay -f rawvideo -pixel_format yuv420p -video_size 640x480 -framerate 25 encoded.yuv

	if err := os.WriteFile("encoded.yuv", bytes.Join(frames, nil), 0644); err != nil {
		log.Fatal(err)
	}

	encoded := make([][]byte, len(frames))

	//Упростив данные вычисли разницу между каждым кадром
	//Во многих случаях пиксели между кадрами меняются незначительно. Следовательно многие длеьты будут небольшими
	//Тогда это можно хранить в небольших дельтах более эффективно

	//Но так как у первого кадра нет предыдущего, поэтом он хранится целиком - называется ключевым кадром
	//Ключевые кадрый могут быть сжаты
	//В данном энкодере делаем нулевой кадр ключевым
	//Остальные кадры буудт отличатся от предыдущего кадра - они называются прогнозируемыми кадрами, P-кадры
	for i := range frames {

		if i == 0 {
			encoded[i] = frames[i]
			continue
		}

		delta := make([]byte, len(frames[i]))

		for j := 0; j < len(delta); j++ {
			delta[j] = frames[i][j] - frames[i-1][j]
		}

		//Теперь тут получили дельта-кадр которые содержит нули
		//Нули легко сжимаются, поэтому будут сжиматься с помощью кодирования длины выполнения
		//Это алгоритм в котором сохраняется количество повторений значения, а затем само значение
		//В совеременных кодеках этой шляпы нет, но для цели сжатия подойдет

		var rle []byte
		for j := 0; j < len(delta); {
			// Count the number of times the current value repeats.
			var count byte
			for count = 0; count < 255 && j+int(count) < len(delta) && delta[j+int(count)] == delta[j]; count++ {
			}

			// Store the count and value.
			rle = append(rle, count)
			rle = append(rle, delta[j])

			j += int(count)
		}

		// Save the RLE frame.
		encoded[i] = rle
	}
	// Это хорошо, у нас 1/4 размера исходного видео. Но мы можем добиться большего.
	// Обратите внимание, что большинство наших самых длинных серий - это серии с нулями. Это потому, что разница
	// между кадрами обычно невелика. У нас есть некоторая гибкость в выборе алгоритма
	// здесь, поэтому, чтобы упростить кодировщик, мы остановимся на использовании алгоритма DEFLATE
	//, который доступен в стандартной библиотеке. Реализация выходит за рамки
	// этой демонстрации.

	rleSize := size(encoded)
	log.Printf("RLE size: %d bytes (%0.2f%% original size)", rleSize, 100*float32(rleSize)/float32(rawSize))

	var deflated bytes.Buffer
	w, err := flate.NewWriter(&deflated, flate.BestCompression)
	if err != nil {
		log.Fatal(err)
	}
	for i := range frames {
		if i == 0 {
			// This is the keyframe, write the raw frame.
			if _, err := w.Write(frames[i]); err != nil {
				log.Fatal(err)
			}
			continue
		}

		delta := make([]byte, len(frames[i]))
		for j := 0; j < len(delta); j++ {
			delta[j] = frames[i][j] - frames[i-1][j]
		}
		if _, err := w.Write(delta); err != nil {
			log.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		log.Fatal(err)
	}

	deflatedSize := deflated.Len()
	log.Printf("DEFLATE size: %d bytes (%0.2f%% original size)", deflatedSize, 100*float32(deflatedSize)/float32(rawSize))

	// Вы заметите, что выполнение шага DEFLATE занимает довольно много времени. В целом, кодировщики, как правило, работают
	// намного медленнее, чем декодеры. Это верно для большинства алгоритмов сжатия, а не только для видеокодеков.
	// Это связано с тем, что кодировщику необходимо проделать большую работу для анализа данных и принятия решений
	// о том, как их сжать. Декодер, с другой стороны, представляет собой простой цикл, который считывает
	// данные и выполняет действия, противоположные кодировщику.
	//
	// На данный момент мы достигли 90%-ной степени сжатия!
	//
	// Кстати, вы, возможно, думаете, что типичное сжатие JPEG составляет 90%, так почему бы не закодировать в JPEG
	// каждый кадр? Хотя это правда, алгоритм, который мы предоставили выше, немного проще, чем JPEG.
	// Мы демонстрируем, что использование преимуществ временной локальности может привести к таким же
	// высоким коэффициентам сжатия, как у JPEG, но с гораздо более простым алгоритмом.
	//
	// Кроме того, алгоритм DEFLATE не использует преимущества двумерности данных
	// и поэтому не так эффективен, как мог бы быть. В реальном мире видеокодеки гораздо более
	// сложнее, чем тот, который мы реализовали здесь. Они используют преимущества двумерности
	// данных, они используют более сложные алгоритмы и оптимизированы для аппаратного обеспечения, на котором они
	// работают. Например, кодек H.264 аппаратно реализован на многих современных графических процессорах.
	//
	// Теперь у нас есть наше закодированное видео. Давайте расшифруем его и посмотрим, что у нас получится.

	// Сначала мы расшифруем поток DEFLATE.
	var inflated bytes.Buffer
	r := flate.NewReader(&deflated)
	if _, err := io.Copy(&inflated, r); err != nil {
		log.Fatal(err)
	}
	if err := r.Close(); err != nil {
		log.Fatal(err)
	}

	//Разделение потока на кадры
	decodedFrames := make([][]byte, 0)
	for {
		frame := make([]byte, width*height*3/2)
		if _, err := io.ReadFull(&inflated, frame); err != nil {
			if err == io.EOF {
				break
			}
			log.Fatal(err)
		}
		decodedFrames = append(decodedFrames, frame)
	}
	// Для каждого кадра, кроме первого, нам нужно добавить предыдущий кадр к дельта-кадру.
	// Это противоположно тому, что мы делали в кодировщике.

	for i := range decodedFrames {
		if i == 0 {
			continue
		}

		for j := 0; j < len(decodedFrames[i]); j++ {
			decodedFrames[i][j] += decodedFrames[i-1][j]
		}
	}

	if err := os.WriteFile("decoded.yuv", bytes.Join(decodedFrames, nil), 0644); err != nil {
		log.Fatal(err)
	}

	// Затем преобразуйте каждый кадр YUV в RGB.

	for i, frame := range decodedFrames {
		Y := frame[:width*height]
		U := frame[width*height : width*height+width*height/4]
		V := frame[width*height+width*height/4:]

		rgb := make([]byte, 0, width*height*3)
		for j := 0; j < height; j++ {
			for k := 0; k < width; k++ {
				y := float64(Y[j*width+k])
				u := float64(U[(j/2)*(width/2)+(k/2)]) - 128
				v := float64(V[(j/2)*(width/2)+(k/2)]) - 128

				r := clamp(y+1.402*v, 0, 255)
				g := clamp(y-0.344*u-0.714*v, 0, 255)
				b := clamp(y+1.772*u, 0, 255)

				rgb = append(rgb, uint8(r), uint8(g), uint8(b))
			}
		}
		decodedFrames[i] = rgb
	}

	// Наконец, запишите декодированное видео в файл.
	//
	// Это видео можно воспроизвести с помощью ffplay:
	//
	// ffplay -f rawvideo -формат пикселя rgb24 -размер видео 384x216 -частота кадров 25 декодировано.rgb24
	//
	out, err := os.Create("decoded.rgb24")
	if err != nil {
		log.Fatal(err)
	}
	defer out.Close()

	for i := range decodedFrames {
		if _, err := out.Write(decodedFrames[i]); err != nil {
			log.Fatal(err)
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
