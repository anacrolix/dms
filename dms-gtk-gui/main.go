package main

import (
	"bitbucket.org/anacrolix/dms/dlna/dms"
	"github.com/anacrolix/go-gtk/gtk"
	"github.com/mattn/go-gtk/gdk"
	"io"
	"log"
	"os"
	"runtime"
)

func main() {
	runtime.LockOSThread()
	gtk.Init(&os.Args)
	gdk.ThreadsInit()

	window := gtk.Window(gtk.GTK_WINDOW_TOPLEVEL)
	window.SetTitle("DMS GUI")
	window.Connect("destroy", gtk.MainQuit)

	vbox := gtk.VBox(false, 0)
	window.Add(vbox)

	hbox := gtk.HBox(false, 0)
	vbox.PackStart(hbox, false, true, 0)

	hbox.PackStart(gtk.Label("Shared directory: "), false, true, 0)

	dialog := gtk.FileChooserDialog(
		"Select directory to share", window, gtk.GTK_FILE_CHOOSER_ACTION_SELECT_FOLDER,
		gtk.GTK_STOCK_CANCEL, gtk.GTK_RESPONSE_CANCEL,
		gtk.GTK_STOCK_OK, gtk.GTK_RESPONSE_ACCEPT)

	button := gtk.FileChooserButtonWithDialog(dialog)
	hbox.Add(button)

	logView := gtk.TextView()
	logView.SetEditable(false)
	logView.ModifyFontEasy("monospace")
	logView.SetWrapMode(gtk.GTK_WRAP_WORD_CHAR)
	logViewScroller := gtk.ScrolledWindow(nil, nil)
	logViewScroller.Add(logView)
	logViewScroller.SetPolicy(gtk.GTK_POLICY_AUTOMATIC, gtk.GTK_POLICY_ALWAYS)
	vbox.PackEnd(logViewScroller, true, true, 0)

	getPath := func() string {
		return button.GetFilename()
	}

	appendToLog := func(text string) {
		var endIter gtk.GtkTextIter
		gdk.ThreadsEnter()
		logBuffer := logView.GetBuffer()
		logBuffer.GetEndIter(&endIter)
		logBuffer.Insert(&endIter, text)
		logView.ScrollToIter(&endIter, 0, false, 0, 0)
		gdk.ThreadsLeave()
	}

	window.ShowAll()
	if dialog.Run() != gtk.GTK_RESPONSE_ACCEPT {
		return
	}
	go func() {
		logReader, logWriter := io.Pipe()
		go func() {
			var buf [128]byte
			for {
				n, err := logReader.Read(buf[:])
				appendToLog(string(buf[:n]))
				if err != nil {
					panic(err)
				}
			}
		}()
		dmsLogger := log.New(logWriter, "", 0)
		log.SetOutput(logWriter)
		dmsServer, err := dms.New(getPath(), dmsLogger)
		if err != nil {
			log.Fatalln(err)
		}
		defer dmsServer.Close()
		runtime.LockOSThread()
		gdk.ThreadsEnter()
		button.Connect("selection-changed", func() {
			dmsServer.SetRootPath(getPath())
		})
		gdk.ThreadsLeave()
		runtime.UnlockOSThread()
		dmsServer.Serve()
	}()
	gtk.Main()
	runtime.UnlockOSThread()
}
