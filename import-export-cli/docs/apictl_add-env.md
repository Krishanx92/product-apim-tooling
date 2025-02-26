## apictl add-env

Add Environment to Config file

### Synopsis

Add new environment and its related endpoints to the config file

```
apictl add-env [flags]
```

### Examples

```
apictl add-env -e production \
--apim  https://localhost:9443 

apictl add-env -e test \
--registration https://idp.com:9443 \
--publisher https://apim.com:9443 \
--devportal  https://apps.com:9443 \
--admin  https://apim.com:9443 \
--token https://gw.com:9443/oauth2/token

apictl add-env -e dev \
--apim https://apim.com:9443 \
--registration https://idp.com:9443 \
--token https://gw.com:9443/oauth2/token

NOTE: The flag --environment (-e) is mandatory.
You can either provide only the flag --apim , or all the other 4 flags (--registration --publisher --devportal --admin) without providing --apim flag.
If you are omitting any of --registration --publisher --devportal --admin flags, you need to specify --apim flag with the API Manager endpoint. In both of the
cases --token flag is optional and use it to specify the gateway token endpoint. This will be used for "apictl get-keys" operation.
```

### Options

```
      --admin string          Admin endpoint for the environment
      --apim string           API Manager endpoint for the environment
      --devportal string      DevPortal endpoint for the environment
  -e, --environment string    Name of the environment to be added
  -h, --help                  help for add-env
      --publisher string      Publisher endpoint for the environment
      --registration string   Registration endpoint for the environment
      --token string          Token endpoint for the environment
```

### Options inherited from parent commands

```
  -k, --insecure   Allow connections to SSL endpoints without certs
      --verbose    Enable verbose mode
```

### SEE ALSO

* [apictl](apictl.md)	 - CLI for Importing and Exporting APIs and Applications

