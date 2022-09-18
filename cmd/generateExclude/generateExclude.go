package main

import (
	"encoding/csv"
	"flag"
	log "github.com/sirupsen/logrus"
	pp "github.com/teamnsrg/profparse"
	"io"
	"math"
	"os"
	"strconv"
	"sync"
)

type Task struct {
	Path string
}

var CompleteCounter int

var Accumulator []bool
var AccumLock sync.Mutex

/*
File,Function,Region Number,Functions in File,Regions in File,Regions in Function,Times File Covered,Percent Times File Covered,Times Function Covered,Percent Times Function Covered,Times Region Covered,Percent Times Region Covered
../../apps/app_lifetime_monitor.cc,_ZN4apps18AppLifetimeMonitor11AddObserverEPNS0_8ObserverE,0,15,55,1,3999,1.0000,3999,1.0000,3999,1.0000
../../apps/app_lifetime_monitor.cc,_ZN4apps18AppLifetimeMonitor13NotifyAppStopERKNSt3__112basic_stringIcNS1_11char_traitsIcEENS1_9allocatorIcEEEE,1,15,55,2,3999,1.0000,0,0.0000,0,0.0000
../../apps/app_lifetime_monitor.cc,_ZN4apps18AppLifetimeMonitor13NotifyAppStopERKNSt3__112basic_stringIcNS1_11char_traitsIcEENS1_9allocatorIcEEEE,2,15,55,2,3999,1.0000,0,0.0000,0,0.0000
../../apps/app_lifetime_monitor.cc,_ZN4apps18AppLifetimeMonitor14NotifyAppStartERKNSt3__112basic_stringIcNS1_11char_traitsIcEENS1_9allocatorIcEEEE,3,15,55,2,3999,1.0000,0,0.0000,0,0.0000
../../apps/app_lifetime_monitor.cc,_ZN4apps18AppLifetimeMonitor14NotifyAppStartERKNSt3__112basic_stringIcNS1_11char_traitsIcEENS1_9allocatorIcEEEE,4,15,55,2,3999,1.0000,0,0.0000,0,0.0000
../../apps/app_lifetime_monitor.cc,_ZN4apps18AppLifetimeMonitor14RemoveObserverEPNS0_8ObserverE,5,15,55,1,3999,1.0000,3998,0.9997,3998,0.9997
../../apps/app_lifetime_monitor.cc,_ZN4apps18AppLifetimeMonitor16OnAppWindowShownEPN10extensions9AppWindowEb,6,15,55,8,3999,1.0000,0,0.0000,0,0.0000
../../apps/app_lifetime_monitor.cc,_ZN4apps18AppLifetimeMonitor16OnAppWindowShownEPN10extensions9AppWindowEb,7,15,55,8,3999,1.0000,0,0.0000,0,0.0000
../../apps/app_lifetime_monitor.cc,_ZN4apps18AppLifetimeMonitor16OnAppWindowShownEPN10extensions9AppWindowEb,8,15,55,8,3999,1.0000,0,0.0000,0,0.0000
*/

var COVERED_ALWAYS_OR_NEVER_EXCLUDE = []string{
	"output/vanilla_region_coverage.csv",
	"output/gremlins_region_coverage.csv",
	//	"output/headless_region_coverage.csv",
}

var COVERED_EVEN_ONCE_EXCLUDE = []string{
	"aboutblank_region_coverage.csv",
}

const float64EqualityThreshold = 1e-6

func almostEqual(a, b float64) bool {
	return math.Abs(a-b) <= float64EqualityThreshold
}

func main() {
	var outfile string
	var covFile string

	flag.StringVar(&outfile, "out", "output/exclude_vector.bv",
		"Path to output file csv")
	flag.StringVar(&covFile, "coverage-file", "coverage.txt",
		"Path to sample text coverage file for metadata generation")
	flag.Parse()

	var err error

	sampleCovMap, _, err := pp.ReadFileToCovMap(covFile)
	if err != nil {
		log.Fatal(err)
	}

	files := 0
	functions := 0
	regions := 0

	Structure := pp.ConvertCovMapToStructure(sampleCovMap)
	for _, v1 := range Structure {
		files += 1
		for _, v2 := range v1 {
			functions += 1
			regions += v2
		}
	}

	alwaysSoFar := make([]bool, regions)
	neverSoFar := make([]bool, regions)
	for i, _ := range alwaysSoFar {
		alwaysSoFar[i] = true
		neverSoFar[i] = true
	}

	for _, fileName := range COVERED_ALWAYS_OR_NEVER_EXCLUDE {
		alwaysCovered, _ := pp.CountCoveredRegions(alwaysSoFar)
		log.Infof("Always covered: %d", alwaysCovered)
		neverCovered, _ := pp.CountCoveredRegions(neverSoFar)
		log.Infof("Never covered: %d", neverCovered)

		alwaysSoFar, neverSoFar, err = getAlwaysAndNeverCoveredVector(alwaysSoFar, neverSoFar, fileName, regions)
		if err != nil {
			log.Fatal(err)
		}
	}

	excludeVector, totalExcluded, err := pp.CombineBVs([][]bool{
		alwaysSoFar, neverSoFar,
	})

	log.Infof("Total Excluded: %d", totalExcluded)

	pp.WriteFileFromBV(outfile, excludeVector)
	alwaysCovered, _ := pp.CountCoveredRegions(alwaysSoFar)
	log.Infof("Always covered: %d", alwaysCovered)
	neverCovered, _ := pp.CountCoveredRegions(neverSoFar)
	log.Infof("Never covered: %d", neverCovered)

	totalExcludedRegions, totalRegions := pp.CountCoveredRegions(excludeVector)
	log.Infof("Excluding a total of %d out of %d regions", totalExcludedRegions, totalRegions)
}

func getAlwaysAndNeverCoveredVector(alwaysSoFar []bool, neverSoFar []bool, regionCoverageFile string, numRegions int) ([]bool, []bool, error) {
	f, err := os.Open(regionCoverageFile)
	if err != nil {
		return nil, nil, err
	}
	reader := csv.NewReader(f)

	header := true

	always := make([]bool, numRegions)
	never := make([]bool, numRegions)

	for {
		record, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			log.Fatal(err)
		}

		if header {
			header = false
			continue
		}

		regionNumber, err := strconv.Atoi(record[2])
		if err != nil {
			log.Fatal(err)
		}

		percentTimesRegionCovered, err := strconv.ParseFloat(record[11], 32)
		if err != nil {
			log.Fatal(err)
		}

		isNeverCovered := almostEqual(percentTimesRegionCovered, 0.0)
		isAlwaysCovered := almostEqual(percentTimesRegionCovered, 1.0)

		always[regionNumber] = alwaysSoFar[regionNumber] && isAlwaysCovered
		never[regionNumber] = neverSoFar[regionNumber] && isNeverCovered
	}

	return always, never, nil
}
