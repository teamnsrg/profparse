#!/usr/bin/env python3

import plotly.express as px
import plotly.graph_objects as go
import numpy as np
import pandas as pd
import sys

if len(sys.argv) != 3:
    print("Need an input file and a depth")
    quit()

df = pd.read_csv(sys.argv[1])
'''
fig = px.sunburst(df, names='names', parents='parents', values='total',
                  color='percentcovered',
                  color_continuous_scale='RdBu',
                  color_continuous_midpoint=0.5)
#fig.show()
'''
names = df['names']
parents = df['parents']
totals = df['total']
percents = df['percentcovered']

fig = go.Figure()

line = go.sunburst.marker.Line()
line.width = 0

fig.add_trace(go.Sunburst(
    ids=df.names,
    labels=df.names,
    parents=df.parents,
    values=df.total,
    hovertemplate='<b>%{label} </b> <br> Parent: %{parent}<br> Total Regions: %{value}<br> Percent: (%{color:.2f})',
    branchvalues="total",
    range_color=[0.0,0.5],
    marker={'colorscale':'rdylgn',
        'colors': df.percentcovered,
        'line': line,
        'showscale':True},
    maxdepth=int(sys.argv[2])))

#fig.layout.height = 1500
#fig.layout.width = 1500

'''
import dash
import dash_core_components as dcc
import dash_html_components as html
app = dash.Dash()
app.layout = html.Div([dcc.Graph(figure=fig),
app.run_server()
    ])
'''


fig.show()

#fig.show()
