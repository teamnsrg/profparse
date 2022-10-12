package main

import (
	"encoding/csv"
	"encoding/json"
	"flag"
	log "github.com/sirupsen/logrus"
	pp "github.com/teamnsrg/profparse"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
)

type Task struct {
	Path string
}

type Result struct {
	Path                         string // Done
	Domain                       string // Done
	Category                     string
	Success                      bool             // Done
	TotalResources               int              // Done
	TotalBlocksCovered           int64            // Done
	TotalResourceBytesDownloaded int64            // Done
	LoadEvent                    bool             // Done
	LoadEventTime                float64          // Done
	BrowserOpenTime              float64          // Done
	GenBlocksCovered             int64            // Done
	SrcBlocksCovered             int64            // Done
	DirBlocksCovered             map[string]int64 // Done
	PercentDirBlocksCovered      map[string]float64
}

type BVRange struct {
	Start int //inclusive
	End   int // exclusive
}

type CloudflareContentCategory struct {
	ID              int    `json:"id,omitempty"`
	SuperCategoryId int    `json:"super_category_id,omitempty"`
	Name            string `json:"name,omitempty"`
}

type CloudflareApplication struct {
	ID   int    `json:"id,omitempty"`
	Name string `json:"name,omitempty"`
}

type CloudflareCategoryEntry struct {
	ContentCategories []CloudflareContentCategory `json:"content_categories"`
	Application       CloudflareApplication       `json:"application"`
}

var CompleteCounter int

var SiteCats map[string]CloudflareCategoryEntry

var ExcludeVector []bool

func main() {
	var covFile string
	var resultsPath string
	var outfile string
	var excludeBVFile string

	flag.StringVar(&resultsPath, "results-path", "results",
		"Path to MIDA results for analysis")
	flag.StringVar(&outfile, "out", "file_coverage.csv",
		"Path to output file csv")
	flag.StringVar(&excludeBVFile, "excludeBV", "",
		"BV file to exclude set regions from analysis")

	flag.Parse()

	var err error

	log.Info("Loading cloudflare categories...")
	SiteCats, err = LoadCloudflareCategories("/home/pmurley/top1mplusVVNN_categories.json")
	if err != nil {
		log.Fatal(err)
	}
	log.Infof("Loaded %d entries for site categories", len(SiteCats))

	CompleteCounter = 0
	log.SetReportCaller(true)
	log.Infof("Begin creating metadata structures by reading %s...", covFile)

	covPaths, err := pp.GetPathsMidaResults(resultsPath, true)
	if err != nil {
		log.Fatal(err)
	}
	log.Infof("Retrieved paths for %d results directories", len(covPaths))
	var resultsPaths []string

	for _, c := range covPaths {
		resultsPaths = append(resultsPaths, c)
	}
	sort.Strings(covPaths)

	taskChan := make(chan Task, 10000)
	resultsChan := make(chan Result, 10000)
	var wg sync.WaitGroup
	var wwg sync.WaitGroup

	wwg.Add(1)
	go writer(resultsChan, &wwg, outfile)

	WORKERS := 28
	for i := 0; i < WORKERS; i++ {
		wg.Add(1)
		go worker(taskChan, resultsChan, &wg)
	}

	for _, resultsPath := range resultsPaths {
		var t Task
		t.Path = resultsPath
		taskChan <- t
	}

	close(taskChan)
	wg.Wait()
	log.Info("Workers finished, starting")

	close(resultsChan)
	wwg.Wait()

	log.Info("Writer has completed")

	numTrials := len(covPaths)
	log.Infof("numTrials: %d", numTrials)
}

func worker(taskChan chan Task, resultChan chan Result, wg *sync.WaitGroup) {
	for task := range taskChan {

		log.Infof("Processing task: %s", task.Path)
		pathsParts := strings.Split(task.Path, "/")
		domain := pathsParts[len(pathsParts)-2]

		metadata, err := pp.LoadMidaMetadata(path.Join(task.Path, "metadata.json"))
		if err != nil {
			log.Error(err)
			continue
		}

		browserOpenedTime := metadata.TaskTiming.BrowserOpen

		loadEventTime := metadata.TaskTiming.LoadEvent
		browserClosedTime := metadata.TaskTiming.BrowserClose

		totalTimeBrowserOpen := browserClosedTime.Sub(browserOpenedTime).Seconds()
		timeToLoadEvent := loadEventTime.Sub(browserOpenedTime).Seconds()

		resourceData, err := pp.LoadMidaResourceData(path.Join(task.Path, "resource_metadata.json"))
		if err != nil {
			log.Error(err)
			continue
		}

		if len(resourceData) < 0 {
			continue
		}

		resourceDir := path.Join(task.Path, "resources")
		dirSize, err := DirSize(resourceDir)
		if err != nil {
			log.Error(err)
		}

		var r Result
		r.Path = task.Path
		r.Domain = domain
		r.Success = metadata.Success
		r.TotalResources = metadata.NumResources
		r.TotalResourceBytesDownloaded = dirSize

		r.BrowserOpenTime = totalTimeBrowserOpen
		if loadEventTime.Year() == 2022 {
			r.LoadEvent = true
			r.LoadEventTime = timeToLoadEvent
		} else {
			r.LoadEvent = false
		}

		if data, ok := SiteCats[domain]; ok && len(data.ContentCategories) == 1 {
			r.Category = data.ContentCategories[0].Name
		} else if len(data.ContentCategories) > 1 {
			r.Category = data.ContentCategories[0].Name
		} else {
			r.Category = "UNKNOWN"
		}

		resultChan <- r

		CompleteCounter += 1
		log.Info(CompleteCounter)
	}
	wg.Done()
}

func writer(resultChan chan Result, wwg *sync.WaitGroup, outfile string) {

	f, err := os.Create(outfile)
	if err != nil {
		log.Fatal(err)
	}

	writer := csv.NewWriter(f)

	header := []string{
		"Domain",
		"Path",
		"Category",
		"Success",
		"Load Event Fired",
		"Load Event Time",
		"Browser Open Time",
		"Total Resources",
		"Total Resource Bytes",
	}

	writer.Write(header)

	for result := range resultChan {
		line := []string{
			result.Domain,
			result.Path,
			result.Category,
			strconv.FormatBool(result.Success),
			strconv.FormatBool(result.LoadEvent),
			strconv.FormatFloat(result.LoadEventTime, 'f', 2, 64),
			strconv.FormatFloat(result.BrowserOpenTime, 'f', 2, 64),
			strconv.Itoa(result.TotalResources),
			strconv.FormatInt(result.TotalResourceBytesDownloaded, 10),
		}

		writer.Write(line)
		writer.Flush()
	}
	f.Close()
	wwg.Done()
}

func DirSize(path string) (int64, error) {
	var size int64
	err := filepath.Walk(path, func(_ string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			size += info.Size()
		}
		return err
	})
	return size, err
}

func LoadCloudflareCategories(filename string) (map[string]CloudflareCategoryEntry, error) {
	jsonBytes, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}

	var entry map[string]CloudflareCategoryEntry
	err = json.Unmarshal(jsonBytes, &entry)
	if err != nil {
		return nil, err
	}

	return entry, nil
}
