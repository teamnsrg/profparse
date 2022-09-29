package main

import (
	"encoding/csv"
	log "github.com/sirupsen/logrus"
	pp "github.com/teamnsrg/profparse"
	"io"
	"os"
	"strconv"
)

/*
File,Function,Region Number,Functions in File,Regions in File,Regions in Function,Times File Covered,Percent Times File Covered,Times Function Covered,Percent Times Function Covered,Times Region Covered,Percent Times Region Covered
../../apps/app_lifetime_monitor.cc,_ZN4apps18AppLifetimeMonitor11AddObserverEPNS0_8ObserverE,0,15,55,1,99754,1.0000,99754,1.0000,99754,1.0000
../../apps/app_lifetime_monitor.cc,_ZN4apps18AppLifetimeMonitor13NotifyAppStopERKNSt3__112basic_stringIcNS1_11char_traitsIcEENS1_9allocatorIcEEEE,1,15,55,2,99754,1.0000,0,0.0000,0,0.0000
../../apps/app_lifetime_monitor.cc,_ZN4apps18AppLifetimeMonitor13NotifyAppStopERKNSt3__112basic_stringIcNS1_11char_traitsIcEENS1_9allocatorIcEEEE,2,15,55,2,99754,1.0000,0,0.0000,0,0.0000
../../apps/app_lifetime_monitor.cc,_ZN4apps18AppLifetimeMonitor14NotifyAppStartERKNSt3__112basic_stringIcNS1_11char_traitsIcEENS1_9allocatorIcEEEE,3,15,55,2,99754,1.0000,0,0.0000,0,0.0000
../../apps/app_lifetime_monitor.cc,_ZN4apps18AppLifetimeMonitor14NotifyAppStartERKNSt3__112basic_stringIcNS1_11char_traitsIcEENS1_9allocatorIcEEEE,4,15,55,2,99754,1.0000,0,0.0000,0,0.0000
../../apps/app_lifetime_monitor.cc,_ZN4apps18AppLifetimeMonitor14RemoveObserverEPNS0_8ObserverE,5,15,55,1,99754,1.0000,99741,0.9998,99741,0.9998
../../apps/app_lifetime_monitor.cc,_ZN4apps18AppLifetimeMonitor16OnAppWindowShownEPN10extensions9AppWindowEb,6,15,55,8,99754,1.0000,0,0.0000,0,0.0000
../../apps/app_lifetime_monitor.cc,_ZN4apps18AppLifetimeMonitor16OnAppWindowShownEPN10extensions9AppWindowEb,7,15,55,8,99754,1.0000,0,0.0000,0,0.0000
../../apps/app_lifetime_monitor.cc,_ZN4apps18AppLifetimeMonitor16OnAppWindowShownEPN10extensions9AppWindowEb,8,15,55,8,99754,1.0000,0,0.0000,0,0.0000
*/

func main() {
	f, err := os.Open("output/100k_region_out.csv")
	if err != nil {
		log.Fatal(err)
	}

	log.SetReportCaller(true)

	reader := csv.NewReader(f)

	header := true

	medianBV := make([]bool, 6389348)

	i := 0
	for {
		i += 1
		record, err := reader.Read()
		if err == io.EOF {
			break
		} else if err != nil {
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

		percentTimesRegionCovered, err := strconv.ParseFloat(record[11], 64)
		if err != nil {
			log.Fatal(err)
		}

		medianBV[regionNumber] = percentTimesRegionCovered >= 0.50

		// log.Infof("Region %d is covered %f percent of the time.", regionNumber, percentTimesRegionCovered)
	}
	log.Info(i)

	pp.WriteFileFromBV("output/100k_median.bv", medianBV)

	bv, err := pp.ReadBVFileToBV("output/100k_median.bv")
	coveredRegions, totalRegions := pp.CountCoveredRegions(bv)
	log.Infof("Median BV covers %d out of %d regions (%f percent)", coveredRegions, totalRegions, float64(coveredRegions)*100.0/float64(totalRegions))
	f.Close()
}
