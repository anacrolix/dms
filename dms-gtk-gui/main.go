package main

import (
	"github.com/anacrolix/go-gtk/gtk"
	"os"
)

func main() {
	gtk.Init(&os.Args)

	window := gtk.Window(gtk.GTK_WINDOW_TOPLEVEL)
	window.SetTitle("DMS GUI")

	vbox := gtk.VBox(false, 0)
	window.Add(vbox)

	hbox := gtk.HBox(false, 0)
	vbox.Add(hbox)
	hbox.Add(gtk.Entry())
	hbox.Add(gtk.FileChooserButton("Share Directory", gtk.GTK_FILE_CHOOSER_ACTION_SELECT_FOLDER))

	logView := gtk.TextView()
	vbox.Add(logView)
	logBuffer := logView.GetBuffer()

	var endIter gtk.GtkTextIter
	logBuffer.GetEndIter(&endIter)
	logBuffer.Insert(&endIter, "hello")

	window.ShowAll()

	gtk.Main()
}
