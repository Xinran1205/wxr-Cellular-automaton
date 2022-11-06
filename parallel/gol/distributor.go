package gol

import (
	"fmt"
	"os"
	"time"
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

func calculateTheNextState(startY, endY, startX, endX int, BBoard [][]uint8, p Params) [][]uint8 {
	// BBoard and copyBoard are always the whole board.
	// partBoard is a part of the board, workerHeight is the height of the partBoard
	workerHeight := endY - startY
	copyBoard := make([][]uint8, p.ImageHeight)
	partBoard := make([][]uint8, workerHeight)
	for i := range copyBoard {
		copyBoard[i] = make([]uint8, p.ImageWidth)
	}
	for i := range partBoard {
		partBoard[i] = make([]uint8, p.ImageWidth)
	}
	for row := 0; row < p.ImageHeight; row++ {
		for col := 0; col < p.ImageWidth; col++ {
			copyBoard[row][col] = BBoard[row][col] / 255
		}
	}
	copyStartY := startY
	for row := 0; row < workerHeight; row++ {
		for col := 0; col < p.ImageWidth; col++ {
			partBoard[row][col] = BBoard[copyStartY][col] / 255
		}
		copyStartY++
	}

	var sum uint8
	newIndex := 0
	for y := startY; y < endY; y++ {
		for x := startX; x < endX; x++ {
			sum = copyBoard[(y+p.ImageHeight-1)%p.ImageHeight][(x+p.ImageWidth-1)%p.ImageWidth] + copyBoard[(y+p.ImageHeight-1)%p.ImageHeight][(x+p.ImageWidth)%p.ImageWidth] + copyBoard[(y+p.ImageHeight-1)%p.ImageHeight][(x+p.ImageWidth+1)%p.ImageWidth] +
				copyBoard[(y+p.ImageHeight)%p.ImageHeight][(x+p.ImageWidth-1)%p.ImageWidth] + copyBoard[(y+p.ImageHeight)%p.ImageHeight][(x+p.ImageWidth+1)%p.ImageWidth] + copyBoard[(y+p.ImageHeight+1)%p.ImageHeight][(x+p.ImageWidth-1)%p.ImageWidth] +
				copyBoard[(y+p.ImageHeight+1)%p.ImageHeight][(x+p.ImageWidth)%p.ImageWidth] + copyBoard[(y+p.ImageHeight+1)%p.ImageHeight][(x+p.ImageWidth+1)%p.ImageWidth]
			if copyBoard[y][x] == 1 {
				if sum < 2 {
					partBoard[newIndex][x] = 0
				} else if sum == 2 || sum == 3 {
					partBoard[newIndex][x] = 255
				} else {
					partBoard[newIndex][x] = 0
				}
			} else {
				if sum == 3 {
					partBoard[newIndex][x] = 255
				} else {
					partBoard[newIndex][x] = 0
				}
			}
		}
		newIndex++
	}
	return partBoard
}

//worker
func worker(startY, endY, startX, endX int, out chan<- [][]uint8, BBoard [][]uint8, p Params) {
	GGraph := calculateTheNextState(startY, endY, startX, endX, BBoard, p)
	out <- GGraph
}

//MakeAliveCellsArray return the array with alive cells
func MakeAliveCellsArray(BBoard [][]uint8, p Params) []util.Cell {
	CurrentAliveArray := make([]util.Cell, 0)
	for i := 0; i < p.ImageHeight; i++ {
		for j := 0; j < p.ImageWidth; j++ {
			if BBoard[i][j] == 255 {
				CurrentAliveArray = append(CurrentAliveArray, util.Cell{X: j, Y: i})
			}
		}
	}
	return CurrentAliveArray
}

//OutPutImage sends the board to the ioOutput
func OutPutImage(BBoard [][]uint8, p Params, c distributorChannels,) {
	c.ioCommand <- ioOutput
	c.ioFilename <- fmt.Sprintf("%vx%vx%v", p.ImageWidth, p.ImageHeight, p.Turns)
	for row := 0; row < p.ImageHeight; row++ {
		for col := 0; col < p.ImageWidth; col++ {
			c.ioOutput <- BBoard[row][col]
		}
	}
}

// distributor divides the work between workers and interacts with other goroutines.
func distributor(p Params, c distributorChannels) {
	c.ioCommand <- ioInput
	c.ioFilename <- fmt.Sprintf("%vx%v", p.ImageWidth, p.ImageHeight)

	//get the board value from the io input
	BBoard := make([][]uint8, p.ImageHeight) //the result board we need
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

	//consider the initial state for the board
	var InitialAliveArray []util.Cell
	InitialAliveArray = MakeAliveCellsArray(BBoard, p)
	aliveCellsNumber := len(InitialAliveArray) // Initial alive cells count
	turn := 0                   // current turn
	for i := range InitialAliveArray {
		c.events <- CellFlipped{turn, InitialAliveArray[i]}
	}

	ticker := time.NewTicker(2 * time.Second)
	c.events <- StateChange{turn, Executing}
	for k := 0; k < p.Turns; k++ {
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
			} else if KeyPresses == 'p' {
				fmt.Println("current turn",turn)
				fmt.Println("Pausing...")
				c.events <- StateChange{turn, Paused}
				for {
					KeyPresses2 := <-c.keyPresses
					if KeyPresses2 == 'p' {
						fmt.Println("Continuing...")
						c.events <- StateChange{turn, Executing}
						break
					}
				}
			}
		case <-ticker.C:
			c.events <- AliveCellsCount{k, aliveCellsNumber}
		default:
		}

		workerHeight := p.ImageHeight / p.Threads
		out := make([]chan [][]uint8, p.Threads+1)
		for i := range out {
			out[i] = make(chan [][]uint8)
		}

		//keep the previous Board for the cellFlipped event
		BeforeBoard := make([][]uint8, p.ImageHeight)
		for i := range BeforeBoard {
			BeforeBoard[i] = make([]uint8, p.ImageWidth)
		}
		for row := 0; row < p.ImageHeight; row++ {
			for col := 0; col < p.ImageWidth; col++ {
				BeforeBoard[row][col] = BBoard[row][col]
			}
		}

		for i := 0; i < p.Threads-1; i++ {
			go worker(i*workerHeight, (i+1)*workerHeight, 0, p.ImageWidth, out[i], BBoard, p)
		}
		go worker((p.Threads-1)*workerHeight, p.ImageHeight, 0, p.ImageWidth, out[p.Threads-1], BBoard, p)

		//build an empty board everytime to receive the part of the board
		EmptyBoard := make([][]uint8, 0)
		for i := range EmptyBoard {
			EmptyBoard[i] = make([]uint8, 0)
		}
		for i := 0; i < p.Threads; i++ {
			part := <-out[i]
			EmptyBoard = append(EmptyBoard, part...)
		}

		//update the value of board
		for row := 0; row < p.ImageHeight; row++ {
			for col := 0; col < p.ImageWidth; col++ {
				BBoard[row][col] = EmptyBoard[row][col]
			}
		}

		var CurrentAliveArray []util.Cell
		CurrentAliveArray = MakeAliveCellsArray(BBoard, p)
		aliveCellsNumber = len(CurrentAliveArray)

		for x := 0; x < p.ImageHeight; x++ {
			for y := 0; y < p.ImageWidth; y++ {
				if BBoard[x][y] != BeforeBoard[x][y] {
					c.events <- CellFlipped{
						CompletedTurns: turn,
						Cell:           util.Cell{X: y, Y: x},
					}
				}
			}
		}

		c.events <- TurnComplete{
			CompletedTurns: turn,
		}
		turn++
	}

	FinalAliveArray := MakeAliveCellsArray(BBoard, p)
	c.events <- FinalTurnComplete{turn, FinalAliveArray}

	OutPutImage(BBoard, p, c)
	c.ioCommand <- ioCheckIdle
	<-c.ioIdle

	c.events <- ImageOutputComplete{turn, fmt.Sprintf("%vx%vx%v", p.ImageWidth, p.ImageHeight, p.Turns)}
	c.events <- StateChange{turn, Quitting}
	close(c.events)
}
