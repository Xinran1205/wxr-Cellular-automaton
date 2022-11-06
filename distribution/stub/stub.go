package stubs

var Handler = "Game.CalculateBBoard"

type Response struct {
	FinalBoard [][]uint8
}

type Request struct {
	InitialBoard [][]uint8
	Threads      int
}
