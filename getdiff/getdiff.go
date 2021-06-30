package main

import (
	"encoding/csv"
	"encoding/json"
	log "github.com/sirupsen/logrus"
	pp "github.com/teamnsrg/profparse"
	"io/ioutil"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
)

type Task struct {
	Path1 string
	Path2 string
}

type Metadata struct {
	NumResources int `json:"num_resources"`
}

var bvMap map[string][]bool
var nrMap map[string]int

func main() {

	resultsPath := "/home/pmurley/go/src/github.com/teamnsrg/profparse/91-100-of-10k"

	log.SetReportCaller(true)
	covPaths, err := pp.GetCovPathsMIDAResults(resultsPath)
	if err != nil {
		log.Fatal(err)
	}

	sort.Strings(covPaths)

	bvMap = make(map[string][]bool)
	nrMap = make(map[string]int)

	for _, covPath := range covPaths {
		bv, err := pp.ReadBVFileToBV(covPath)
		if err != nil {
			log.Error(err)
			continue
		}

		metaPath := strings.Replace(covPath, "coverage/coverage.bv", "metadata.json", 1)
		data, err := ioutil.ReadFile(metaPath)
		if err != nil {
			log.Error(err)
			continue
		}
		var meta Metadata
		err = json.Unmarshal(data, &meta)
		if err != nil {
			log.Error(err)
			continue
		}

		nrMap[covPath] = meta.NumResources

		bvMap[covPath] = bv
	}

	taskChan := make(chan Task, 10000)
	writerChan := make(chan []string, 10000)
	var wg sync.WaitGroup
	var wwg sync.WaitGroup

	wwg.Add(1)
	go writer(writerChan, &wwg)

	WORKERS := 5
	for i := 0; i < WORKERS; i++ {
		wg.Add(1)
		go worker(taskChan, writerChan, &wg)
	}

	for i, path1 := range covPaths {
		for _, path2 := range covPaths[i:] {
			var t Task
			t.Path1 = path1
			t.Path2 = path2
			taskChan <- t
		}
	}
	close(taskChan)
	wg.Wait()
	close(writerChan)
	wwg.Wait()
}

func worker(taskChan chan Task, writerChan chan []string, wg *sync.WaitGroup) {

	for task := range taskChan {
		diff, err := pp.DiffTwoBVs(bvMap[task.Path1], bvMap[task.Path2])
		if err != nil {
			log.Error(err)
		}

		parts := strings.Split(task.Path1, "/")
		url1 := parts[9]
		uuid1 := parts[10]

		parts = strings.Split(task.Path2, "/")
		url2 := parts[9]
		uuid2 := parts[10]

		result := []string{url1, uuid1, strconv.Itoa(nrMap[task.Path1]), url2, uuid2, strconv.Itoa(nrMap[task.Path2]), strconv.Itoa(diff)}
		writerChan <- result

	}
	wg.Done()
}

func writer(writerChan chan []string, wg *sync.WaitGroup) {
	f, err := os.Create("diffs.csv")
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
	wg.Done()
}
