// Copyright 2021 The Graphene Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bufio"
	"encoding/csv"
	"flag"
	"fmt"
	"image/color/palette"
	"io"
	"math"
	"os"
	"strconv"
	"strings"
	tm "time"

	"go.bug.st/serial.v1"
	"gonum.org/v1/plot"
	"gonum.org/v1/plot/plotter"
	"gonum.org/v1/plot/vg"
	"gonum.org/v1/plot/vg/draw"
)

// ReadMe is the README
const ReadMe = `
# A free energy device
Below device is a piece [pyrolytic graphite](https://en.wikipedia.org/wiki/Pyrolytic_carbon) sandwiched between two [neodymium magnets](https://en.wikipedia.org/wiki/Neodymium_magnet).

## 1st experiment
![1st experiment](20221115-104833.jpg?raw=true)

## 2nd experiment the next day
![2nd experiment](20221116-020655.jpg?raw=true)

Didn't work a well because the pyrolytic graphite was damaged, but this image proves the first image wasn't the result of an IR reflection.

# Long running experiments

`

var (
	// FlagRead read from the meter
	FlagRead = flag.String("read", "", "read from the meter")
)

func main() {
	flag.Parse()

	if *FlagRead != "" {
		mode := &serial.Mode{
			BaudRate: 9600,
			Parity:   serial.NoParity,
			DataBits: 8,
			StopBits: serial.OneStopBit,
		}
		port, err := serial.Open(*FlagRead, mode)
		if err != nil {
			panic(err)
		}
		defer port.Close()
		reader := bufio.NewReader(port)
		writer := bufio.NewWriter(port)
		writer.WriteString("QD 0\r\n")
		writer.Flush()

		buffer := make([]byte, 5)
		_, err = io.ReadFull(reader, buffer)
		if err != nil {
			panic(err)
		}
		fmt.Println(buffer)
		if buffer[0] != '0' || buffer[1] != '\r' || buffer[2] != 'Q' || buffer[3] != 'D' || buffer[4] != ',' {
			panic("invalid response")
		}

		buffer = make([]byte, 7)
		_, err = io.ReadFull(reader, buffer)
		if err != nil {
			panic(err)
		}
		count := uint64(0)
		count |= uint64(buffer[0])
		count |= uint64(buffer[1]) << 8
		count++
		fmt.Println("count", count)
		time := uint64(0)
		time = uint64(buffer[2])
		time |= uint64(buffer[3]) << 8
		time |= uint64(buffer[4]) << 16
		time |= uint64(buffer[5]) << 24
		fmt.Println("time", time)
		clockSet := uint64(buffer[6])
		fmt.Println("clock set", clockSet)

		writer.WriteString("QD 1\r\n")
		writer.Flush()
		buffer, i := make([]byte, 8), 0
		for uint64(i) < count {
			fmt.Println("reading", i, "of", count)
			_, err := io.ReadFull(reader, buffer)
			fmt.Println(buffer)
			if err != nil {
				panic(err)
			}
			fmt.Println(buffer[7] & (1 << 7))
			tm.Sleep(100 * tm.Millisecond)
			i++
		}
		return
	}
	output, err := os.Create("README.md")
	if err != nil {
		panic(err)
	}
	defer output.Close()
	_, err = output.WriteString(ReadMe)
	if err != nil {
		panic(err)
	}
	process(true, "Pyrolytic graphite experiment 1", "log1.csv", output)
}

func process(fluke bool, title, log string, output *os.File) {
	fmt.Fprintf(output, "### %s - %s\n", title, log)
	input, err := os.Open(log)
	if err != nil {
		panic(err)
	}
	defer input.Close()
	decoder := csv.NewReader(input)
	decoder.FieldsPerRecord = -1
	record, err := decoder.Read()
	if err != nil {
		panic(err)
	}
	t1i, t2i := 1, 2
	if fluke {
		t2i = 3
		for record[0] != "Reading" {
			record, _ = decoder.Read()
		}
	}
	fmt.Println(log, record)
	sum, count := 0.0, 0
	points1, points2 := make(plotter.XYs, 0, 8), make(plotter.XYs, 0, 8)
	record, err = decoder.Read()
	for err == nil {
		t1, err1 := strconv.ParseFloat(strings.TrimSpace(record[t1i]), 64)
		if err1 != nil {
			panic(err1)
		}
		t2, err1 := strconv.ParseFloat(strings.TrimSpace(record[t2i]), 64)
		if err1 != nil {
			panic(err1)
		}
		fmt.Println(log, t1, t2)
		sum += math.Abs(t1 - t2)
		points1 = append(points1, plotter.XY{X: float64(count), Y: float64(t1)})
		points2 = append(points2, plotter.XY{X: float64(count), Y: float64(t2)})
		count++
		record, err = decoder.Read()
	}
	fmt.Println("average=", sum/float64(count))
	fmt.Fprintf(output, "* average=%f\n", sum/float64(count))

	deviation := func(values plotter.XYs) float64 {
		a, b, count := 0.0, 0.0, 0
		for _, value := range values {
			a += value.Y * value.Y
			b += value.Y
			count++
		}
		return math.Sqrt((a - b*b/float64(count)) / float64(count))
	}
	sigma1 := deviation(points1)
	sigma2 := deviation(points2)
	fmt.Println("sigma1=", sigma1)
	fmt.Println("sigma2=", sigma2)
	average := func(values plotter.XYs) float64 {
		sum, count := 0.0, 0
		for _, value := range values {
			sum += value.Y
			count++
		}
		return sum / float64(count)
	}
	average1 := average(points1)
	average2 := average(points2)
	fmt.Println("average1=", average1)
	fmt.Println("average2=", average2)
	corr, count := 0.0, 0
	for i := range points1 {
		corr += (points1[i].Y - average1) * (points2[i].Y - average2)
		count++
	}
	corr /= float64(count) * sigma1 * sigma2
	fmt.Println("corr=", corr)
	fmt.Fprintf(output, "* corr=%f\n", corr)
	p := plot.New()

	p.Title.Text = "temperature vs time"
	p.X.Label.Text = "time"
	p.Y.Label.Text = "temperature"

	scatter, err := plotter.NewScatter(points1)
	if err != nil {
		panic(err)
	}
	scatter.GlyphStyle.Radius = vg.Length(1)
	scatter.GlyphStyle.Shape = draw.CircleGlyph{}
	scatter.GlyphStyle.Color = palette.WebSafe[0x00F]
	p.Add(scatter)

	scatter, err = plotter.NewScatter(points2)
	if err != nil {
		panic(err)
	}
	scatter.GlyphStyle.Radius = vg.Length(1)
	scatter.GlyphStyle.Shape = draw.CircleGlyph{}
	scatter.GlyphStyle.Color = palette.WebSafe[0x030]

	p.Add(scatter)

	image := strings.Replace(log, ".bin", ".png", 1)
	err = p.Save(8*vg.Inch, 8*vg.Inch, image)
	if err != nil {
		panic(err)
	}
	fmt.Fprintf(output, "\n![%s](%s?raw=true)\n\n", log, image)
}