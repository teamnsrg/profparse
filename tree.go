package profparse

import (
	"encoding/csv"
	"os"
	"sort"
	"strconv"
	"strings"
)

func GetTreeSummary(covMap map[string]map[string][]bool, level int) map[string]CovSummary {
	tree := make(map[string]CovSummary)
	for fileName := range covMap {
		totalRegionsInFile := 0
		coveredRegionsInFile := 0
		totalFunctionsInFile := 0
		coveredFunctionsInFile := 0

		for funcName := range covMap[fileName] {
			totalFunctionsInFile += 1
			funcCovered := false
			for _, regionCovered := range covMap[fileName][funcName] {
				totalRegionsInFile += 1
				if regionCovered {
					coveredRegionsInFile += 1
					funcCovered = true
				}
			}
			if funcCovered {
				coveredFunctionsInFile += 1
			}
		}

		parts := strings.Split(fileName, "/")
		if parts[0] == ".." && parts[1] == ".." {
			parts = parts[2:]
		} else if parts[0] == "gen" {
			parts = parts[1:]
		}
		for i := 0; i < len(parts) && (level <= 0 || i < level-1); i++ {
			seg := strings.Join(parts[:i+1], "/")
			if _, ok := tree[seg]; !ok {
				tree[seg] = CovSummary{
					TotalRegions:   0,
					CoveredRegions: 0,
					PercentCovered: 0,
				}
			}

			regionSummary := tree[seg]
			regionSummary.TotalRegions += totalRegionsInFile
			regionSummary.CoveredRegions += coveredRegionsInFile
			regionSummary.PercentCovered = float64(regionSummary.CoveredRegions) / float64(regionSummary.TotalRegions)
			tree[seg] = regionSummary
		}
	}

	return tree
}

func ConvertFileCoverageToTree(fc map[string]CovSummary) map[string]int {
	tree := make(map[string]int)
	for k, v := range fc {
		parts := strings.Split(k, "/")
		if parts[0] == ".." && parts[1] == ".." {
			parts = parts[2:]
		} else if parts[0] == "gen" {
			parts = parts[1:]
		}

		for i := 0; i < len(parts); i++ {
			seg := strings.Join(parts[:i+1], "/")
			if _, ok := tree[seg]; !ok {
				tree[seg] = 0
			}
			tree[seg] += v.CoveredRegions
		}
	}

	return tree
}

func WriteTreeToFile(tree map[string]CovSummary, fileName string) error {
	f, err := os.Create(fileName)
	if err != nil {
		return err
	}
	defer f.Close()

	writer := csv.NewWriter(f)

	// Write header
	err = writer.Write([]string{
		"Path",
		"Regions Covered",
		"Total Regions",
		"Percent Covered",
	})

	keys := make([]string, 0, len(tree))
	for k := range tree {
		keys = append(keys, k)
	}

	sort.Strings(keys)

	for _, k := range keys {
		err = writer.Write([]string{
			k,
			strconv.Itoa(tree[k].CoveredRegions),
			strconv.Itoa(tree[k].TotalRegions),
			strconv.FormatFloat(tree[k].PercentCovered, 'f', 2, 64),
		})
		if err != nil {
			return err
		}
	}

	writer.Flush()
	return nil
}
