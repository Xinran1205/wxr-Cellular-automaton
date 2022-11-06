package gol

import (
	"fmt"
	"net/rpc"
	"os"
	"sync"
	"time"
	stubs "uk.ac.bris.cs/gameoflife/stub"
	"uk.ac.bris.cs/gameoflife/util"
)

type distributorChannels struct {
	events     chan<- Event
	ioCommand  chan<- ioCommand
	ioIdle     <-chan bool
	ioFilename chan<- string
	ioOutput   chan<- uint8
	ioInput    <-chan uint8
	keyPresses <-chan rune
}

var m *sync.Mutex
var aliveCellsArray []util.Cell

func CalculateAliveCell(Board [][]uint8) []util.Cell {
	var finalAliveCellSlice []util.Cell
	for i := 0; i < len(Board); i++ {
		for j := 0; j < len(Board); j++ {
			if Board[i][j] != 0 {
				finalAliveCellSlice = append(finalAliveCellSlice, util.Cell{X: j, Y: i})
			}
		}
	}
	return finalAliveCellSlice
}

func OutPutImage(BBoard [][]uint8, p Params, c distributorChannels) {
	c.ioCommand <- ioOutput
	c.ioFilename <- fmt.Sprintf("%vx%vx%v", p.ImageWidth, p.ImageHeight, p.Turns)
	for row := 0; row < p.ImageHeight; row++ {
		for col := 0; col < p.ImageWidth; col++ {
			c.ioOutput <- BBoard[row][col]
		}
	}
}

func makeCall(client rpc.Client, BBoard [][]uint8, p Params) [][]uint8 {
	request := stubs.Request{InitialBoard: BBoard, Threads: p.Threads}
	response := new(stubs.Response)
	client.Call(stubs.Handler, request, response)
	return response.FinalBoard
}

// distributor divides the work between workers and interacts with other goroutines.
func distributor(p Params, c distributorChannels) {
	keyPause := make(chan int, 1)
	m = new(sync.Mutex)
	c.ioCommand <- ioInput
	c.ioFilename <- fmt.Sprintf("%vx%v", p.ImageWidth, p.ImageHeight)

	BBoard := make([][]uint8, p.ImageHeight)
	for i := range BBoard {
		BBoard[i] = make([]uint8, p.ImageWidth)
	}
	var element uint8
	for row := 0; row < p.ImageHeight; row++ {
		for col := 0; col < p.ImageWidth; col++ {
			element = <-c.ioInput
			BBoard[row][col] = element
		}
	}
	turn := 0
	var InitialAliveArray []util.Cell
	InitialAliveArray = CalculateAliveCell(BBoard)
	for i := range InitialAliveArray {
		c.events <- CellFlipped{turn, InitialAliveArray[i]}
	}
	CurrentAlive := len(InitialAliveArray)
	server := "127.0.0.1:8030"
	client, _ := rpc.Dial("tcp", server)
	defer client.Close()

	pausing := 0
	ticker := time.NewTicker(2 * time.Second)
	c.events <- StateChange{turn, Executing}
	go func() {
		for {
			select {
			case KeyPresses := <-c.keyPresses:
				if KeyPresses == 's' {
					OutPutImage(BBoard, p, c)
					c.events <- ImageOutputComplete{turn, fmt.Sprintf("%vx%vx%v", p.ImageWidth, p.ImageHeight, p.Turns)}
				} else if KeyPresses == 'q' {
					OutPutImage(BBoard, p, c)
					c.events <- ImageOutputComplete{turn, fmt.Sprintf("%vx%vx%v", p.ImageWidth, p.ImageHeight, p.Turns)}
					c.events <- StateChange{turn, Quitting}
					os.Exit(001)
				} else if KeyPresses == 'k' {
					OutPutImage(BBoard, p, c)
					c.events <- ImageOutputComplete{turn, fmt.Sprintf("%vx%vx%v", p.ImageWidth, p.ImageHeight, p.Turns)}
					c.events <- StateChange{turn, Quitting}
					os.Exit(001)
				} else if KeyPresses == 'p' {
					fmt.Println("current turn", turn)
					fmt.Println("Pausing...")
					pausing = 1
					c.events <- StateChange{turn, Paused}
					for {
						KeyPresses2 := <-c.keyPresses
						if KeyPresses2 == 'p' {
							pausing = 0
							keyPause <- 1
							fmt.Println("Continuing...")
							c.events <- StateChange{turn, Executing}
							break
						}
					}
				}
			case <-ticker.C:
				if turn < p.Turns && pausing == 0 {
					m.Lock()
					c.events <- AliveCellsCount{turn, CurrentAlive}
					m.Unlock()
				}
			}
		}
	}()

	aliveCellsArray = CalculateAliveCell(BBoard)
	for i := 0; i < p.Turns; i++ {
		//if we press p, it will enter this if statement and wait until this channel receives value
		if pausing == 1 {
			<-keyPause
		}

		NewBoard := makeCall(*client, BBoard, p)

		for x := 0; x < p.ImageHeight; x++ {
			for y := 0; y < p.ImageWidth; y++ {
				if NewBoard[x][y] != BBoard[x][y] {
					c.events <- CellFlipped{
						CompletedTurns: turn,
						Cell:           util.Cell{X: y, Y: x},
					}
				}
			}
		}

		for row := 0; row < len(BBoard); row++ {
			for col := 0; col < len(BBoard); col++ {
				BBoard[row][col] = NewBoard[row][col]
			}
		}

		aliveCellsArray = CalculateAliveCell(BBoard)
		CurrentAlive = len(aliveCellsArray)

		c.events <- TurnComplete{
			CompletedTurns: turn,
		}
		turn++
	}

	c.events <- FinalTurnComplete{p.Turns, aliveCellsArray}

	OutPutImage(BBoard, p, c)
	c.ioCommand <- ioCheckIdle
	<-c.ioIdle

	c.events <- ImageOutputComplete{turn, fmt.Sprintf("%vx%vx%v", p.ImageWidth, p.ImageHeight, p.Turns)}
	c.events <- StateChange{turn, Quitting}
	close(c.events)
}
