host := "localhost:1338"

run *args:
    GOPPROF=http godo -v -- . {{ args }}

browse-root:
    curl -s -X POST http://{{ host }}/ctl \
        -H "Content-Type: text/xml" \
        -H 'SOAPAction: "urn:schemas-upnp-org:service:ContentDirectory:1#Browse"' \
        --data-binary @testdata/browse-root.xml
