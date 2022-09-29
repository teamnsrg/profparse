package main

import (
	"encoding/csv"
	log "github.com/sirupsen/logrus"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"
)

/*
names,parents,covered,total,percentcovered
angle,,0,0,NaN
angle/angle_commit.h,angle,0,0,NaN
apps,,450,645,0.698
apps/app_lifetime_monitor.cc,apps,0,55,0.000
apps/app_lifetime_monitor.h,apps,5,5,1.000
apps/app_lifetime_monitor_factory.cc,apps,7,7,1.000
apps/app_restore_service.cc,apps,35,35,1.000
apps/app_restore_service_factory.cc,apps,0,6,0.000
apps/browser_context_keyed_service_factories.cc,apps,9,9,1.000
apps/launcher.cc,apps,231,231,1.000
apps/saved_files_service.cc,apps,71,139,0.511
*/
func main() {

	TREE_ONE := "output/amiunique-fp-median-tree.csv"
	TREE_TWO := "output/amiunique-front-median-tree.csv"

	log.SetReportCaller(true)

	header := true

	treeOneMap := make(map[string][]string)
	treeTwoMap := make(map[string][]string)

	f, err := os.Open(TREE_ONE)
	if err != nil {
		log.Fatal(err)
	}
	reader := csv.NewReader(f)
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

		treeOneMap[record[0]] = record
	}
	f.Close()

	f, err = os.Open(TREE_TWO)
	if err != nil {
		log.Fatal(err)
	}
	reader = csv.NewReader(f)
	i = 0
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

		treeTwoMap[record[0]] = record
	}

	g, err := os.Create("output/fpTree.csv")
	if err != nil {
		log.Fatal(err)
	}
	writer := csv.NewWriter(g)
	writer.Write([]string{
		"names", "parents", "covered", "total", "percentcovered",
	})

	keys := make([]string, 0)
	for k := range treeOneMap {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, k := range keys {
		oneRecord := treeOneMap[k]
		if _, ok := treeTwoMap[k]; !ok {
			log.Fatalf("Key %s present in tree one but not tree two")
		}

		twoRecord := treeTwoMap[k]

		if oneRecord[0] != twoRecord[0] || oneRecord[1] != twoRecord[1] || oneRecord[3] != twoRecord[3] {
			log.Fatalf(strings.Join(append(oneRecord, twoRecord...), " "))
		}

		oneCovered, err := strconv.Atoi(oneRecord[2])
		if err != nil {
			log.Fatal(err)
		}

		twoCovered, err := strconv.Atoi(twoRecord[2])
		if err != nil {
			log.Fatal(err)
		}

		total, err := strconv.Atoi(oneRecord[3])
		if err != nil {
			log.Fatal(err)
		}

		writer.Write([]string{
			oneRecord[0],
			oneRecord[1],
			strconv.Itoa(oneCovered - twoCovered),
			oneRecord[3],
			strconv.FormatFloat((float64(oneCovered-twoCovered)/float64(total)+1.0)/2.0, 'f', 5, 64),
		})

	}

	g.Close()
	f.Close()
}
