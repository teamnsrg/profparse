package main

import (
	"flag"
	log "github.com/sirupsen/logrus"
	pp "github.com/teamnsrg/profparse"
	"path"
	"sort"
	"sync"
)

type Task struct {
	Path string
}

type Result struct {
	Path      string
	FinalTree map[string]int
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
var SortedFiles []string
var SortedFuncs map[string][]string
var DenominatorFileCoverageMap map[string]int
var DenominatorTree map[string]int
var TotalBVs int
var RegionCovCounts []int
var RegionCountLock sync.Mutex

var ExcludeBV []bool

func main() {
	var covFile string
	var resultsPath string
	var controlResultsPath string
	var outfiledir string
	var excludeBVPath string

	flag.StringVar(&covFile, "coverage-file", "",
		"Path to sample text coverage file for metadata generation")
	flag.StringVar(&resultsPath, "results-path", "",
		"Path to MIDA results for analysis")
	flag.StringVar(&controlResultsPath, "control-results-path", "",
		"Path to CONTROL MIDA results for analysis")
	flag.StringVar(&excludeBVPath, "exclude-bv", "",
		"Path to BV file to use for region exclusion")
	flag.StringVar(&outfiledir, "out", "output",
		"Path to output file directory")

	flag.Parse()

	var positiveSiteDirectories = []string{
		//"/data3/nsrg/mida_results/fingerprinting/browserleaks.com-canvas",
		"/data3/nsrg/mida_results/fingerprinting/amiunique.org-fp",
		"/data3/nsrg/mida_results/1m-4k-2022-3x-vanilla/walmart.com",
		"/data3/nsrg/mida_results/1m-4k-2022-3x-vanilla/vmware.com",
		"/data3/nsrg/mida_results/VVNN-100k/3m.com",
		"/data3/nsrg/mida_results/1m-4k-2022-3x-vanilla/autodesk.com",
	}

	var negativeSiteDirectories = []string{
		"/data3/nsrg/mida_results/fingerprinting/browserleaks.com",
		//"/data3/nsrg/mida_results/fingerprinting/browserleaks.com-webrtc",
		//"/data3/nsrg/mida_results/fingerprinting/browserleaks.com-webgl",
		//"/data3/nsrg/mida_results/fingerprinting/browserleaks.com-fonts",
		"/data3/nsrg/mida_results/fingerprinting/amiunique.org",
		"/data3/nsrg/mida_results/fingerprinting/amiunique.org-faq",
		"/data3/nsrg/mida_results/canvas/www.w3schools.com-html-html5_canvas.asp",
		"/data3/nsrg/mida_results/1m-4k-2022-3x-vanilla/harvard.edu",
		"/data3/nsrg/mida_results/1m-4k-2022-3x-vanilla/illinois.edu",
		"/data3/nsrg/mida_results/1m-4k-2022-3x-vanilla/mozilla.org",
		"/data3/nsrg/mida_results/1m-4k-2022-3x-vanilla/state.gov",
	}

	var err error
	CompleteCounter = 0
	log.SetReportCaller(true)
	log.Infof("Begin creating metadata structures by reading %s...", covFile)

	MetaMap, _, err = pp.ReadCovMetadata(covFile)
	if err != nil {
		log.Fatal(err)
	}

	sampleCovMap, _, err := pp.ReadFileToCovMap(covFile)
	if err != nil {
		log.Fatal(err)
	}

	files := 0
	functions := 0
	regions := 0

	sampleBV := pp.ConvertCovMapToBools(sampleCovMap)
	RegionCovCounts = make([]int, len(sampleBV))

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

	fileNames := make([]string, 0)
	for k := range Structure {
		fileNames = append(fileNames, k)
	}
	sort.Strings(fileNames)

	for _, k := range fileNames {
		start = currentIndex
		funcNames := make([]string, 0)
		for funcName := range Structure[k] {
			funcNames = append(funcNames, funcName)
		}
		sort.Strings(funcNames)

		for _, funcName := range funcNames {
			currentIndex += Structure[k][funcName]
		}
		end = currentIndex
		FilenameToBVIndices[k] = BVRange{
			Start: start,
			End:   end,
		}
	}

	excludeBV := make([]bool, len(sampleBV))
	if excludeBVPath != "" {
		excludeBV, err = pp.ReadBVFileToBV(excludeBVPath)
		if err != nil {
			log.Fatal(err)
		}
	}

	ExcludeBV = excludeBV

	excludedRegions, _ := pp.CountCoveredRegions(excludeBV)
	log.Infof("Excluding %d regions from '%s'", excludedRegions, excludeBVPath)

	SortedFiles = make([]string, 0)
	SortedFuncs = make(map[string][]string)
	DenominatorFileCoverageMap = make(map[string]int)
	for k := range Structure {
		SortedFiles = append(SortedFiles, k)
		funcs := make([]string, 0)
		DenominatorFileCoverageMap[k] = 0
		fileRange := FilenameToBVIndices[k]
		for i := fileRange.Start; i < fileRange.End; i++ {
			if !excludeBV[i] {
				DenominatorFileCoverageMap[k] += 1
			}
		}
		for k2 := range Structure[k] {
			funcs = append(funcs, k2)
		}
		sort.Strings(funcs)
		SortedFuncs[k] = funcs
	}
	sort.Strings(SortedFiles)

	log.Infof("Finished parsing metadata")
	log.Infof("  - Total Files: %d", files)
	log.Infof("  - Total Functions: %d", functions)
	log.Infof("  - Total Code Regions: %d\n", regions)

	siteCovPaths := make([]string, 0)

	for _, p := range positiveSiteDirectories {
		covPaths, err := pp.GetCovPathsSite(p)
		if err != nil {
			log.Fatal(err)
		}

		siteCovPaths = append(siteCovPaths, covPaths...)
	}

	sort.Strings(siteCovPaths)

	bvs := make([][]bool, 0)
	for _, covPath := range siteCovPaths {
		bv, err := pp.ReadBVFileToBV(covPath)
		if err != nil {
			log.Error(err)
			continue
		}
		bvs = append(bvs, bv)
	}

	medianBV, err := pp.GetThresholdBV(bvs, 0.85)
	if err != nil {
		log.Fatal(err)
	}

	siteControlCovPaths := make([]string, 0)

	for _, p := range negativeSiteDirectories {
		controlCovPaths, err := pp.GetCovPathsSite(p)
		if err != nil {
			log.Fatal(err)
		}
		siteControlCovPaths = append(siteControlCovPaths, controlCovPaths...)

	}

	sort.Strings(siteControlCovPaths)

	bvs = make([][]bool, 0)
	for _, covPath := range siteControlCovPaths {
		bv, err := pp.ReadBVFileToBV(covPath)
		if err != nil {
			log.Error(err)
			continue
		}
		bvs = append(bvs, bv)
	}

	controlMedianBV, err := pp.GetThresholdBV(bvs, 0.01)
	if err != nil {
		log.Fatal(err)
	}

	compareMaskExclude := make([]bool, len(medianBV))
	comparedRegions := 0
	for i := range medianBV {
		if excludeBV[i] {
			compareMaskExclude[i] = true
		} else if !medianBV[i] {
			compareMaskExclude[i] = true
		} else if controlMedianBV[i] == medianBV[i] {
			compareMaskExclude[i] = true
		} else {
			comparedRegions += 1
		}
	}
	log.Infof("Compared Regions: %d", comparedRegions)

	err = pp.WriteFileFromBV(path.Join(outfiledir, "compareMaskExclude.bv"), compareMaskExclude)
	if err != nil {
		log.Error(err)
	}
	err = pp.WriteFileFromBV(path.Join(outfiledir, "compareMaskCovered.bv"), medianBV)
	if err != nil {
		log.Error(err)
	}
}
