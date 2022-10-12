#!/usr/bin/env python3

import csv
import pandas as pd

df = pd.read_csv('output/comparedBVFiles.csv')
print(df)

df.groupby(['File'])['File'].count().sort_values(ascending=False).to_csv( 'out.csv')
