package main

import (
	"encoding/csv"
	"encoding/json"
	"flag"
	log "github.com/sirupsen/logrus"
	pp "github.com/teamnsrg/profparse"
	"net/url"
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
	Success                      bool  // Done
	TotalResources               int   // Done
	TotalBlocksCovered           int64 // Done
	TotalResourceBytesDownloaded int64 // Done
	TotalScripts                 int64
	TotalDocuments               int64
	TotalImages                  int64
	TotalFonts                   int64
	TotalStylesheets             int64
	TotalXHRs                    int64
	TotalOrigins                 int64
	TotalOriginsScripts          int64
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

var MetaMap map[string]map[string][]pp.CodeRegion
var Structure map[string]map[string]int
var BVIndexToCodeRegionMap map[int]pp.CodeRegion
var FilenameToBVIndices map[string]BVRange
var CompleteCounter int
var SortedFiles []string
var DenominatorFileCoverageMap map[string]int
var DenominatorTree map[string]int
var FileCoverage map[string][]float64
var FileCovCounts map[string]int
var FileCovCountLock sync.Mutex

var SiteCats map[string]CloudflareCategoryEntry

var ExcludeVector []bool

var DirectoriesOfInterest = []string{}

//var DirectoriesOfInterest = []string{
//	"net/websockets",
//}

func main() {
	var covFile string
	var resultsPath string
	var outfile string
	var excludeBVFile string

	flag.StringVar(&covFile, "coverage-file", "coverage.txt",
		"Path to sample text coverage file for metadata generation")
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

	var metaProps pp.CovMapProperties
	MetaMap, metaProps, err = pp.ReadCovMetadata(covFile)
	if err != nil {
		log.Fatal(err)
	}

	log.Infof("MetaMap Files: %d, Funcs: %d, Regions: %d", metaProps.NumFiles, metaProps.NumFunctions, metaProps.NumRegions)

	FileCovCounts = make(map[string]int)

	sampleCovMap, props, err := pp.ReadFileToCovMap(covFile)
	if err != nil {
		log.Fatal(err)
	}

	log.Infof("CovMap Files: %d, Funcs: %d, Regions: %d", props.NumFiles, props.NumFunctions, props.NumRegions)

	files := 0
	functions := 0
	regions := 0

	sampleBV := pp.ConvertCovMapToBools(sampleCovMap)

	log.Info("Reading BV to exclude")
	if excludeBVFile != "" {
		ExcludeVector, err = pp.ReadBVFileToBV(excludeBVFile)
		if err != nil {
			log.Fatal(err)
		}
		log.Infof("Read exclude vector (length: %d)", len(ExcludeVector))
	} else {
		ExcludeVector = make([]bool, len(sampleBV))
		log.Infof("Created empty exclude vector (length: %d)", len(ExcludeVector))
	}

	Structure = pp.ConvertCovMapToStructure(sampleCovMap)
	for _, v1 := range Structure {
		files += 1
		for _, v2 := range v1 {
			functions += 1
			regions += v2
		}
	}
	BVIndexToCodeRegionMap = pp.GenerateBVIndexToCodeRegionMap(Structure, MetaMap)
	log.Infof("BVIndexToCodeRegionMap Length: %d", len(BVIndexToCodeRegionMap))

	// Build FilenameToBVIndices Map
	FilenameToBVIndices = make(map[string]BVRange)
	currentIndex := 0
	start := 0
	end := 0
	totalRegions := 0

	fileNames := make([]string, 0)
	for fileName := range Structure {
		fileNames = append(fileNames, fileName)
	}
	sort.Strings(fileNames)

	for _, fileName := range fileNames {
		start = currentIndex
		funcNames := make([]string, 0)
		for funcName := range Structure[fileName] {
			funcNames = append(funcNames, funcName)
		}

		for _, funcName := range funcNames {
			currentIndex += Structure[fileName][funcName]
		}
		end = currentIndex
		FilenameToBVIndices[fileName] = BVRange{
			Start: start,
			End:   end,
		}
		totalRegions += end - start
	}

	log.Infof("Total Files, Regions for FilenameToBVIndices: %d, %d", len(FilenameToBVIndices), totalRegions)

	FileCoverage = make(map[string][]float64)

	log.Infof("Finished parsing metadata")
	log.Infof("  - Total Files: %d", files)
	log.Infof("  - Total Functions: %d", functions)
	log.Infof("  - Total Code Regions: %d\n", regions)

	covPaths, err := pp.GetCovPathsMIDAResults(resultsPath, true)
	if err != nil {
		log.Fatal(err)
	}
	log.Infof("Retrieved paths for %d results directories", len(covPaths))
	var resultsPaths []string

	for _, c := range covPaths {
		resultsPaths = append(resultsPaths, strings.TrimSuffix(c, "coverage/coverage.bv"))
	}

	sort.Strings(covPaths)
	SortedFiles = make([]string, 0)
	for k := range Structure {
		SortedFiles = append(SortedFiles, k)
	}
	sort.Strings(SortedFiles)

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
		domain := pathsParts[len(pathsParts)-3]

		bv, err := pp.ReadBVFileToBV(path.Join(task.Path, "coverage", "coverage.bv"))
		if err != nil {
			log.Error(err)
			continue
		}

		metadata, err := pp.LoadMidaMetadata(path.Join(task.Path, "metadata.json"))
		if err != nil {
			log.Error(err)
			continue
		}

		var r Result

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

		var scripts int64 = 0
		var fonts int64 = 0
		var documents int64 = 0
		var images int64 = 0
		var stylesheets int64 = 0
		var xhrs int64 = 0
		var others int64 = 0

		originsAll := make(map[string]bool)
		originsScripts := make(map[string]bool)
		for _, entry := range resourceData {
			if len(entry.Requests) == 0 || entry.Response == nil {
				continue
			}

			if entry.Response.Type.String() == "Script" {
				scripts += 1
			} else if entry.Response.Type.String() == "Document" {
				documents += 1
			} else if entry.Response.Type.String() == "XHR" {
				xhrs += 1
			} else if entry.Response.Type.String() == "Image" {
				images += 1
			} else if entry.Response.Type.String() == "Stylesheet" {
				stylesheets += 1
			} else if entry.Response.Type.String() == "Font" {
				fonts += 1
			} else if entry.Response.Type.String() == "Other" {
				others += 1
			}

			url, err := url.Parse(entry.Response.Response.URL)
			if err != nil {
				continue
			}

			originsAll[url.Host] = true
			if entry.Response.Type.String() == "Script" {
				originsScripts[url.Host] = true
			}
		}

		r.TotalDocuments = documents
		r.TotalScripts = scripts
		r.TotalImages = images
		r.TotalStylesheets = stylesheets
		r.TotalFonts = fonts
		r.TotalXHRs = xhrs

		r.TotalOrigins = int64(len(originsAll))
		r.TotalOriginsScripts = int64(len(originsScripts))

		var blocksCovered int64 = 0
		var genBlocksCovered int64 = 0
		var srcBlocksCovered int64 = 0

		totalBlocksPerDir := make(map[string]int64)
		coveredBlocksPerDir := make(map[string]int64)

		for _, dir := range DirectoriesOfInterest {
			totalBlocksPerDir[dir] = 0
			coveredBlocksPerDir[dir] = 0
		}

		for _, fileName := range SortedFiles {
			isGen := strings.HasPrefix(fileName, "gen/")
			isSrc := strings.HasPrefix(fileName, "../../")

			isDirMap := make(map[string]bool)
			for _, dir := range DirectoriesOfInterest {
				if strings.HasPrefix(fileName, "gen/"+dir) || strings.HasPrefix(fileName, "../../"+dir) {
					isDirMap[dir] = true
				} else {
					isDirMap[dir] = false
				}
			}

			indices := FilenameToBVIndices[fileName]
			fileCov := 0
			for i := indices.Start; i < indices.End; i++ {
				if ExcludeVector[i] {
					continue
				}
				for _, dir := range DirectoriesOfInterest {
					if isDirMap[dir] {
						totalBlocksPerDir[dir] += 1
						if bv[i] {
							coveredBlocksPerDir[dir] += 1
							if isGen {
								genBlocksCovered += 1
							}
							if isSrc {
								srcBlocksCovered += 1
							}
						}
					}
				}

				if bv[i] {
					fileCov += 1
					blocksCovered += 1
				}
			}

			FileCovCountLock.Lock()
			if _, ok := FileCoverage[fileName]; !ok {
				FileCoverage[fileName] = make([]float64, 0)
			}
			FileCoverage[fileName] = append(FileCoverage[fileName], float64(fileCov)/float64(indices.End-indices.Start))

			FileCovCountLock.Unlock()
		}

		resourceDir := path.Join(task.Path, "resources")
		dirSize, err := DirSize(resourceDir)
		if err != nil {
			log.Error(err)
		}

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

		r.TotalBlocksCovered = blocksCovered
		r.GenBlocksCovered = genBlocksCovered
		r.SrcBlocksCovered = srcBlocksCovered
		r.DirBlocksCovered = coveredBlocksPerDir

		r.PercentDirBlocksCovered = make(map[string]float64)
		for _, dir := range DirectoriesOfInterest {
			r.PercentDirBlocksCovered[dir] = float64(r.DirBlocksCovered[dir]) / float64(totalBlocksPerDir[dir])
			log.Infof("Dir: %s, Covered: %d, Total: %d, Percent: %.02f", dir,
				r.DirBlocksCovered[dir], totalBlocksPerDir[dir], float64(r.DirBlocksCovered[dir])/float64(totalBlocksPerDir[dir]))
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
		"Total Documents",
		"Total Scripts",
		"Total Images",
		"Total Stylesheets",
		"Total Fonts",
		"Total XHRs",
		"Total Origins",
		"Total Script Origins",
		"Total Blocks Covered",
		"Gen Blocks Covered",
		"Src Blocks Covered",
	}

	for _, dir := range DirectoriesOfInterest {
		header = append(header, "RegionsCovered: "+dir)
		header = append(header, "PercentCovered: "+dir)
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
			strconv.FormatInt(result.TotalDocuments, 10),
			strconv.FormatInt(result.TotalScripts, 10),
			strconv.FormatInt(result.TotalImages, 10),
			strconv.FormatInt(result.TotalStylesheets, 10),
			strconv.FormatInt(result.TotalFonts, 10),
			strconv.FormatInt(result.TotalXHRs, 10),
			strconv.FormatInt(result.TotalOrigins, 10),
			strconv.FormatInt(result.TotalOriginsScripts, 10),
			strconv.FormatInt(result.TotalBlocksCovered, 10),
			strconv.FormatInt(result.GenBlocksCovered, 10),
			strconv.FormatInt(result.SrcBlocksCovered, 10),
		}

		for _, dir := range DirectoriesOfInterest {
			line = append(line, strconv.FormatInt(result.DirBlocksCovered[dir], 10))
			line = append(line, strconv.FormatFloat(result.PercentDirBlocksCovered[dir], 'f', 4, 64))
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
