package main

import (
	"encoding/csv"
	"flag"
	log "github.com/sirupsen/logrus"
	pp "github.com/teamnsrg/profparse"
	"os"
	"sort"
	"sync"
)

type Task struct {
	Path string
}

type Result struct {
	FileCoverageMap map[string]int
}

type BVRange struct {
	Start int //inclusive
	End   int // exclusive
}

var MetaMap map[string]map[string][]pp.CodeRegion
var Structure map[string]map[string]int
var BVIndexToCodeRegionMap map[int]pp.CodeRegion
var FilenameToBVIndices map[string]BVRange
var CompleteCounter int

func main() {
	var covFile string
	var resultsPath string
	var outfile string

	flag.StringVar(&covFile, "coverage-file", "",
		"Path to sample text coverage file for metadata generation")
	flag.StringVar(&resultsPath, "results-path", "",
		"Path to MIDA results for analysis")
	flag.StringVar(&outfile, "out", "results.csv",
		"Path to output file")

	flag.Parse()

	var err error
	CompleteCounter = 0
	log.SetReportCaller(true)
	log.Infof("Begin creating metadata structures by reading %s...", covFile)

	MetaMap, err = pp.ReadCovMetadata(covFile)
	if err != nil {
		log.Fatal(err)
	}

	sampleCovMap, err := pp.ReadFileToCovMap(covFile)
	if err != nil {
		log.Fatal(err)
	}

	files := 0
	functions := 0
	regions := 0

	Structure = pp.ConvertCovMapToStructure(sampleCovMap)
	for _, v1 := range Structure {
		files += 1
		for _, v2 := range v1 {
			functions += 1
			regions += v2
		}
	}
	BVIndexToCodeRegionMap = pp.GenerateBVIndexToCodeRegionMap(Structure, MetaMap)

	// Build FilenameToBVIndices Map
	FilenameToBVIndices = make(map[string]BVRange)
	currentIndex := 0
	start := 0
	end := 0
	for k, v := range Structure {
		start = currentIndex
		for _, numBlocks := range v {
			currentIndex += numBlocks
		}
		end = currentIndex
		FilenameToBVIndices[k] = BVRange{
			Start: start,
			End:   end,
		}
	}

	log.Infof("Finished parsing metadata")
	log.Infof("  - Total Files: %d", files)
	log.Infof("  - Total Functions: %d", functions)
	log.Infof("  - Total Code Regions: %d\n", regions)

	covPaths, err := pp.GetCovPathsMIDAResults(resultsPath)
	if err != nil {
		log.Fatal(err)
	}

	sort.Strings(covPaths)

	taskChan := make(chan Task, 10000)
	writerChan := make(chan []string, 10000)
	var wg sync.WaitGroup
	var wwg sync.WaitGroup

	wwg.Add(1)
	go writer(writerChan, outfile, &wwg)

	WORKERS := 5
	for i := 0; i < WORKERS; i++ {
		wg.Add(1)
		go worker(taskChan, writerChan, &wg)
	}

	for _, path := range covPaths {
		var t Task
		t.Path = path
		taskChan <- t
	}

	close(taskChan)
	wg.Wait()
	close(writerChan)
	wwg.Wait()
}

func worker(taskChan chan Task, writerChan chan []string, wg *sync.WaitGroup) {
	for task := range taskChan {
		_, err := pp.ReadBVFileToBV(task.Path)
		if err != nil {
			log.Error(err)
			continue
		}
		CompleteCounter += 1
		log.Info(CompleteCounter)
	}
	wg.Done()
}

func writer(writerChan chan []string, outfile string, wwg *sync.WaitGroup) {
	f, err := os.Create(outfile)
	if err != nil {
		log.Fatal(err)
	}
	writer := csv.NewWriter(f)

	written := 0

	for row := range writerChan {
		writer.Write(row)
		writer.Flush()

		written += 1
		if written%1000 == 0 {
			log.Infof("Written %d lines", written)
		}
	}
	f.Close()
	wwg.Done()
}
