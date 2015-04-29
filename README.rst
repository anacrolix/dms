dms
===

.. image:: https://codeship.com/projects/e0fc22e0-d084-0132-7215-42f608f62b99/status?branch=master
 :target: https://codeship.com/projects/77002

dms is a UPnP DLNA Digital Media Server. It runs from the terminal, and serves
content directly from the filesystem from the working directory, or the path
given. The SSDP component will broadcast and respond to requests on all
available network interfaces.

dms advertises and serves the raw files, in addition to alternate transcoded
streams when it's able, such as mpeg2 PAL-DVD and WebM for the Chromecast. It
will also provide thumbnails where possible.

dms uses ``ffprobe``/``avprobe`` to get media data such as bitrate and duration,
and ``ffmpeg``/``avconv`` for video transoding.

.. image:: https://lh3.googleusercontent.com/-z-zh7AzObGo/UEiWni1cQPI/AAAAAAAAASI/DRw9IoMMiNs/w497-h373/2012%2B-%2B1

Installing
==========

To run dms, assuming ``$GOPATH`` and Go have been configured already::

    $ go get github.com/anacrolix/dms
    $ $GOPATH/bin/dms

Known Compatible Players and Renderers
======================================

 * Probably all Panasonic Viera TVs.
 * Android's BubbleUPnP and AirWire
 * Chromecast
 * VLC
