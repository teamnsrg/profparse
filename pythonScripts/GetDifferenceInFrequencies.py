#!/usr/bin/env python3

import pandas as pd

positives = pd.read_csv('output/positive_frequency.csv')
negatives = pd.read_csv('output/negative_frequency.csv')

joined = positives.set_index("Region Index").join(negatives.set_index("Region Index"), lsuffix=' Positive', rsuffix=' Negative')

joined['Number of Times Covered Positive'] = joined['Number of Times Covered Positive'].fillna(0.0)
joined['Total Trials Positive'] = joined['Total Trials Positive'].fillna(7887.0)
joined['Percent Covered Positive'] = joined['Percent Covered Positive'].fillna(0.0)

joined['Number of Times Covered Negative'] = joined['Number of Times Covered Negative'].fillna(0.0)
joined['Total Trials Negative'] = joined['Total Trials Negative'].fillna(7887.0)
joined['Percent Covered Negative'] = joined['Percent Covered Negative'].fillna(0.0)

joined['Difference'] = joined['Percent Covered Positive'] - joined['Percent Covered Negative']
print(joined)

joined.to_csv('output/differences_in_frequency.csv')