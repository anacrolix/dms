package main

import (
	"log"
	"os"
	"runtime"

	"github.com/mattn/go-gtk/gdk"
	"github.com/mattn/go-gtk/gtk"

	"github.com/anacrolix/dms/dlna/dms"
)

func main() {
	runtime.LockOSThread()
	gtk.Init(&os.Args)
	gdk.ThreadsInit()

	window := gtk.NewWindow(gtk.WINDOW_TOPLEVEL)
	window.SetTitle("DMS GUI")
	window.Connect("destroy", gtk.MainQuit)

	vbox := gtk.NewVBox(false, 0)
	window.Add(vbox)

	hbox := gtk.NewHBox(false, 0)
	vbox.PackStart(hbox, false, true, 0)

	hbox.PackStart(gtk.NewLabel("Shared directory: "), false, true, 0)

	dialog := gtk.NewFileChooserDialog(
		"Select directory to share", window, gtk.FILE_CHOOSER_ACTION_SELECT_FOLDER,
		gtk.STOCK_CANCEL, gtk.RESPONSE_CANCEL,
		gtk.STOCK_OK, gtk.RESPONSE_ACCEPT)

	button := gtk.NewFileChooserButtonWithDialog(dialog)
	hbox.Add(button)

	logView := gtk.NewTextView()
	logView.SetEditable(false)
	logView.ModifyFontEasy("monospace")
	logView.SetWrapMode(gtk.WRAP_WORD_CHAR)
	logViewScroller := gtk.NewScrolledWindow(nil, nil)
	logViewScroller.Add(logView)
	logViewScroller.SetPolicy(gtk.POLICY_AUTOMATIC, gtk.POLICY_ALWAYS)
	vbox.PackEnd(logViewScroller, true, true, 0)

	getPath := func() string {
		return button.GetFilename()
	}

	window.ShowAll()
	if dialog.Run() != gtk.RESPONSE_ACCEPT {
		return
	}
	go func() {
		dmsServer := dms.Server{
			RootObjectPath: getPath(),
		}
		if err := dmsServer.Serve(); err != nil {
			log.Fatalln(err)
		}
		defer dmsServer.Close()
		runtime.LockOSThread()
		gdk.ThreadsEnter()
		button.Connect("selection-changed", func() {
			dmsServer.RootObjectPath = getPath()
		})
		gdk.ThreadsLeave()
		runtime.UnlockOSThread()
		dmsServer.Serve()
	}()
	gtk.Main()
	runtime.UnlockOSThread()
}
