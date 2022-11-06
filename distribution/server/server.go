package main

import (
	"errors"
	"flag"
	"net"
	"net/rpc"
	stubs "uk.ac.bris.cs/gameoflife/stub"
)

func calculateTheNextState (startY , endY , startX , endX int,Board [][]uint8)[][]uint8{
	workerHeight := endY-startY
	copyBoard := make([][]uint8, len(Board))
	partBoard := make([][]uint8, workerHeight)
	for i := range copyBoard {
		copyBoard[i] = make([]uint8, len(Board))
	}
	for i := range partBoard {
		partBoard[i] = make([]uint8, len(Board))
	}
	for row := 0; row < len(Board); row++ {
		for col := 0; col < len(Board); col++ {
			copyBoard[row][col] =  Board[row][col] / 255
		}
	}
	copyStartY := startY
	for row := 0; row < workerHeight; row++ {
		for col := 0; col < len(Board); col++ {
			partBoard[row][col] = Board[copyStartY][col] / 255
		}
		copyStartY++
	}

	var sum uint8
	newIndex :=0
	for y := startY; y < endY; y++{
		for x := startX; x < endX; x++ {
			sum = copyBoard[(y+len(Board)-1)%len(Board)][(x+len(Board)-1)%len(Board)] + copyBoard[(y+len(Board)-1)%len(Board)][(x+len(Board))%len(Board)] + copyBoard[(y+len(Board)-1)%len(Board)][(x+len(Board)+1)%len(Board)] +
				copyBoard[(y+len(Board))%len(Board)][(x+len(Board)-1)%len(Board)] + copyBoard[(y+len(Board))%len(Board)][(x+len(Board)+1)%len(Board)] + copyBoard[(y+len(Board)+1)%len(Board)][(x+len(Board)-1)%len(Board)] +
				copyBoard[(y+len(Board)+1)%len(Board)][(x+len(Board))%len(Board)] + copyBoard[(y+len(Board)+1)%len(Board)][(x+len(Board)+1)%len(Board)]
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

func worker(startY, endY, startX, endX int, out chan<- [][]uint8, Board [][]uint8,) {
	GGraph := calculateTheNextState (startY, endY, startX, endX, Board)
	out <- GGraph
}

func handleWorker(Board [][]uint8,threads int)[][]uint8{

		EmptyBoard := make([][]uint8, 0)
		for i := range EmptyBoard {
			EmptyBoard[i] = make([]uint8, 0)
		}

		workerHeight := len(Board) / threads
		out := make([]chan [][]uint8, threads+1)
		for i := range out {
			out[i] = make(chan [][]uint8)
		}

		for i := 0; i < threads-1; i++ {
			go worker(i*workerHeight, (i+1)*workerHeight, 0, len(Board), out[i], Board)
		}
		go worker((threads-1)*workerHeight, len(Board), 0, len(Board), out[threads-1], Board)

		for i := 0; i < threads; i++ {
			part := <-out[i]
			EmptyBoard = append(EmptyBoard, part...)
		}

		for row := 0; row < len(Board); row++ {
			for col := 0; col < len(Board); col++ {
				Board[row][col] = EmptyBoard[row][col]
			}
		}
	return Board
}

type Game struct{}

func (s *Game) CalculateBBoard(req stubs.Request, res *stubs.Response) (err error) {
	if req.InitialBoard == nil {
		err = errors.New("A Board must be specified")
		return
	}
	res.FinalBoard = handleWorker(req.InitialBoard,req.Threads)
	return
}

func main(){
	pAddr := flag.String("port","8030","Port to listen on")
	flag.Parse()
	rpc.Register(&Game{})
	listener, _ := net.Listen("tcp", ":"+*pAddr)
	defer listener.Close()
	rpc.Accept(listener)
}