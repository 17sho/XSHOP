import json
import urllib.request

with urllib.request.urlopen('http://127.0.0.1:18080/api/v1/public/config') as response:
    config = json.load(response)['data']

assert config['app_version'] == 'v1.4.8', config['app_version']
