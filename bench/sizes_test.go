package main

import "testing"

func TestCorpusSizes(t *testing.T) {
	for _, p := range Corpus() {
		t.Logf("%-7s total=%7d B head=%6d B (%s)", p.Name, len(p.HTML), p.HeadSize, p.Label)
	}
}
