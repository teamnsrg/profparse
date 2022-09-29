#!/usr/bin/env python3

import csv

f = open('output/100k_crawl_out.csv','r')
reader = csv.reader(f)

header = True
c = {}
for row in reader:
    if header:
        header = False
        continue
    bvPath = row[0]
    regionsCovered = int(row[1])

    parts = bvPath.split('/')
    k = '/'.join([parts[5], parts[6]])
    c[k] = regionsCovered

f.close()

f = open('output/VVNN_100k-metadata_only.csv','r')
reader = csv.reader(f)
g = open('output/VVNN_meta_coverage.csv','w')
writer = csv.writer(g)

header = True
for row in reader:
    if header:
        writer.writerow(row + ['Regions Covered'])
        header = False
        continue
    path = row[1]
    parts = path.split('/')
    k = '/'.join([parts[5], parts[6]])
    if k not in c:
        print(k, ' is not present')
        continue

    writer.writerow(row + [c[k]])
f.close()
g.close()