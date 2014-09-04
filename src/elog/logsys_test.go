package elog

import (
	"testing"
)

func Benchmark_WriteLog(b *testing.B) {

	InitLog(INFO)
	for i := 0; i < b.N; i++ {

		LogInfo("This is test:%d", i)
	}
	Flush()
}

func Benchmark_TimeWriteLog(b *testing.B) {

	b.StopTimer()
	InitLog(INFO)
	b.StartTimer()
	for i := 0; i < b.N; i++ {

		LogSys("This is test:%d", i)
	}
	//Flush()
}
