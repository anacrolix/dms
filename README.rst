dms
===

dms is a UPnP DLNA Digital Media Server. It runs from the terminal, and serves
content directly from the filesystem from the working directory, or the path
given. The SSDP component will broadcast and respond to requests on all
available network interfaces.

dms advertises and serves the raw files, in addition to alternate transcoded
streams when it's able, such as mpeg2 PAL-DVD and WebM for the Chromecast. It
will also provide thumbnails where possible.

dms also supports serving dynamic streams (e.g. a live rtsp stream) generated 
on the fly with the help of an external application (e.g. ffmpeg).

dms uses ``ffprobe``/``avprobe`` to get media data such as bitrate and duration, ``ffmpeg``/``avconv`` for video transoding, and ``ffmpegthumbnailer`` for generating thumbnails when browsing. These commands must be in the ``PATH`` given to ``dms`` or the features requiring them will be disabled.

.. image:: https://i.imgur.com/qbHilI7.png

Installing
==========

Assuming ``$GOPATH`` and Go have been configured already::

    $ go get github.com/anacrolix/dms

Ensure ``ffmpeg``/``avconv`` and/or ``ffmpegthumbnailer`` are in the ``PATH`` if the features depending on them are desired.

To run::

    $ "$GOPATH"/bin/dms

Running DMS using Docker
========================

`dms` is distributed as Docker Image. Serve Media in `/mediadirectory` using `dms`:

.. code-block:: bash

   docker pull ghcr.io/anacrolix/dms:latest
   docker run -d --network host -v /mediadirectory:/dmsdir ghcr.io/anacrolix/dms:latest

Running DMS as a systemd service
=================================

A sample systemd `.service` file has been `provided <helpers/systemd/dms.service>`_ to assist in running DMS as a system service.

Running DMS as a FreeBSD service
================================

Install the `provided <helpers/bsd/dms>`_ service file to /etc/rc.d or /usr/local/etc/rc.d
add ``dms_enable="YES"``, and optionally ``dms_root="/path/to/my/media"`` and ``dms_user="myuser"`` to your /etc/rc.conf

Known Compatible Players and Renderers
======================================

 * Probably all Panasonic Viera TVs.
 * Android's BubbleUPnP and AirWire
 * Chromecast
 * VLC
 * LG Smart TVs, with varying success.
 * Roku devices
 * Apple TV 4K via VLC and 8player
 * iOS VLC and 8player


Usage of dms:
=====================

.. list-table:: Usage
   :widths: auto
   :header-rows: 1

   * - parameter
     - description
   * - ``-allowDynamicStreams``
     - turns on support for `.dms.json` files in the path
   * - ``-allowedIps string``
     - allowed ip of clients, separated by comma
   * - ``-config string``
     - json configuration file
   * - ``-deviceIcon string``
     - device icon
   * - ``-fFprobeCachePath string``
     - path to FFprobe cache file (default "/home/efreak/.dms-ffprobe-cache")
   * - ``-forceTranscodeTo string``
     - force transcoding to certain format, supported: 'chromecast', 'vp8'
   * - ``-friendlyName string``
     - server friendly name
   * - ``-http string``
     - http server port (default ":1338")
   * - ``-ifname string``
     - specific SSDP network interface
   * - ``-ignoreHidden``
     - ignore hidden files and directories
   * - ``-ignoreUnreadable``
     - ignore unreadable files and directories
   * - ``-logHeaders``
     - log HTTP headers
   * - ``-noProbe``
     - disable media probing with ffprobe
   * - ``-noTranscode``
     - disable transcoding
   * - ``-notifyInterval duration``
     - interval between SSPD announces (default 30s)
   * - ``-path string``
     - browse root path
   * - ``-stallEventSubscribe``
     - workaround for some bad event subscribers
   * - ``-transcodeLogPattern``
     - pattern where to write transcode logs to. The ``[tsname]`` placeholder is replaced with the name of the item currently being played. The default is ``$HOME/.dms/log/[tsname]``. You may turn off transcode logging entirely by setting it to ``/dev/null``. You may log to stderr by setting ``/dev/stderr``.

Dynamic streams
===============
DMS supports "dynamic streams" generated on the fly. This feature can be activated with the
``-allowDynamicStreams`` command line flag and can be configured by placing special metadata
files in your content directory.
The name of these metadata files ends with ``.dms.json``, their structure is `documented here <https://pkg.go.dev/github.com/anacrolix/dms/dlna/dms>`_.

An example::

    {
      "Title": "My awesome webcam",
      "Resources": [
         {
            "MimeType": "video/webm",
            "Command": "ffmpeg -i rtsp://10.6.8.161:554/Streaming/Channels/502/ -c:v copy -c:a copy -movflags +faststart+frag_keyframe+empty_moov -f matroska -"
         }
      ]
    }
