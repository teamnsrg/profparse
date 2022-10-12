#!/usr/bin/env python3

import pandas as pd

similarities = pd.read_csv('output/compare_mask_similarities.csv')
correlate = pd.read_csv('output/100k_crawl_out.csv')

result = similarities.set_index('Results Path').join(correlate.set_index('Results Path'))
print(result)

print(result['Regions RegionsCovered'].describe())

positives = result[result['Percent'] > 0.4]
negatives = result[(result['Percent'] < 0.3)]
negatives = negatives[(negatives['Regions RegionsCovered'] > 765637)].sample(len(positives.index))

print(positives)

print(negatives)

print(positives['Regions RegionsCovered'].describe())
print(negatives['Regions RegionsCovered'].describe())

positives.to_csv('output/positives.csv', columns=[], header=False)
negatives.to_csv('output/negatives.csv', columns=[], header=False)
