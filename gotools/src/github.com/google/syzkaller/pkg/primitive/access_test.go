package primitive_test

import (
	"testing"

	"github.com/google/syzkaller/pkg/primitive"
)

var testAcc = []primitive.Access{
	{Timestamp: 0, Inst: 1},
	{Timestamp: 3, Inst: 2},
	{Timestamp: 2, Inst: 3, Thread: 1},
	{Timestamp: 6, Inst: 3, Thread: 0},
	{Timestamp: 1, Inst: 5},
}

var serializedAcc = []primitive.Access{
	{Timestamp: 0, Inst: 1},
	{Timestamp: 1, Inst: 5},
	{Timestamp: 2, Inst: 3, Thread: 1},
	{Timestamp: 3, Inst: 2},
	{Timestamp: 6, Inst: 3, Thread: 0},
}

func TestSerialAccessAdd(t *testing.T) {
	serial := primitive.SerialAccess{}
	for _, acc := range testAcc {
		serial.Add(acc)
	}
	if len(serial) != len(serializedAcc) {
		t.Errorf("wrong length, expected %v, got %v", len(serializedAcc), len(serial))
	}
	for i, acc := range serial {
		if acc.Inst != serializedAcc[i].Inst {
			t.Errorf("wrong #%d, expected %v, got %v", i, serializedAcc[i].Inst, acc.Inst)
		}
	}
}

func TestSerializeAccess(t *testing.T) {
	serial := primitive.SerializeAccess(testAcc)
	if len(serial) != len(serializedAcc) {
		t.Errorf("wrong length, expected %v, got %v", len(serializedAcc), len(serial))
	}
	for i, acc := range serial {
		if acc.Inst != serializedAcc[i].Inst {
			t.Errorf("wrong #%d, expected %v, got %v", i, serializedAcc[i].Inst, acc.Inst)
		}
	}
}

func TestFindIndex(t *testing.T) {
	serial := primitive.SerialAccess{}
	for _, acc := range testAcc {
		serial.Add(acc)
	}
	for i, acc := range serializedAcc {
		if idx := serial.FindIndex(acc); idx != i {
			t.Errorf("wrong, expected %v, got %v", i, idx)
		}
	}
}

func TestSerialAccessFindForeachThread(t *testing.T) {
	serial := primitive.SerializeAccess(testAcc)
	found := serial.FindForeachThread(3, 1)
	if len(found) != 2 {
		t.Errorf("wrong length, expected 2, got %v", len(found))
	}
	if found[0].Timestamp != 2 || found[1].Timestamp != 6 {
		t.Errorf("wrong %v", found)
	}
	found = serial.FindForeachThread(2, 1)
	if len(found) != 1 {
		t.Errorf("wrong length, expected 1, got %v", len(found))
	}
	if found[0].Timestamp != 3 {
		t.Errorf("wrong %v", found)
	}
}
