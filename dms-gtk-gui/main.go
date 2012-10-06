package main

import (
	"github.com/anacrolix/go-gtk/gtk"
	"github.com/mattn/go-gtk/gdk"
	"io"
	"log"
	"os"
	"os/exec"
)

var (
	child    *exec.Cmd
	lastPath string
	logView  *gtk.GtkTextView
)

func appendToLog(text string) {
	var endIter gtk.GtkTextIter
	gdk.ThreadsEnter()
	logBuffer := logView.GetBuffer()
	logBuffer.GetEndIter(&endIter)
	logBuffer.Insert(&endIter, text)
	// logBuffer.GetEndIter(&endIter)
	logView.ScrollToIter(&endIter, 0, false, 0, 0)
	// gdk.Flush()
	gdk.ThreadsLeave()
}

func killChild() {
	if child != nil {
		child.Process.Kill()
	}
}

func restartChild(path string) {
	killChild()
	if path == "" {
		return
	}
	child = exec.Command("dms", "-path", path)
	r, w, err := os.Pipe()
	if err != nil {
		panic(err)
	}
	child.Stdout = w
	child.Stderr = w
	go func() {
		defer r.Close()
		var buf [4096]byte
		for {
			n, err := r.Read(buf[:])
			appendToLog(string(buf[:n]))
			if err == io.EOF {
				break
			}
			if err != nil {
				panic(err)
			}
		}
	}()
	lastPath = path
	if err := child.Start(); err != nil {
		panic(err)
	}
	w.Close()
	go func() {
		if err := child.Wait(); err != nil {
			appendToLog("server terminated: " + err.Error() + "\n")
		}
	}()
}

func main() {
	gtk.Init(&os.Args)
	gdk.ThreadsInit()

	window := gtk.Window(gtk.GTK_WINDOW_TOPLEVEL)
	window.SetTitle("DMS GUI")
	window.Connect("destroy", gtk.MainQuit)

	vbox := gtk.VBox(false, 0)
	window.Add(vbox)

	hbox := gtk.HBox(false, 0)
	vbox.PackStart(hbox, false, true, 0)

	hbox.PackStart(gtk.Label("Share directory: "), false, true, 0)

	dialog := gtk.FileChooserDialog(
		"Media directory", window, gtk.GTK_FILE_CHOOSER_ACTION_SELECT_FOLDER,
		gtk.GTK_STOCK_CANCEL, gtk.GTK_RESPONSE_CANCEL,
		gtk.GTK_STOCK_OPEN, gtk.GTK_RESPONSE_ACCEPT)

	button := gtk.FileChooserButtonWithDialog(dialog)
	hbox.Add(button)

	changed := func() {
		path := button.GetFilename()
		log.Println("path changed to:", path)
		if path != lastPath {
			restartChild(path)
		}
	}

	logView = gtk.TextView()
	logView.SetEditable(false)
	logView.ModifyFontEasy("monospace")
	logView.SetWrapMode(gtk.GTK_WRAP_WORD_CHAR)
	logViewScroller := gtk.ScrolledWindow(nil, nil)
	logViewScroller.Add(logView)
	logViewScroller.SetPolicy(gtk.GTK_POLICY_AUTOMATIC, gtk.GTK_POLICY_ALWAYS)
	vbox.PackEnd(logViewScroller, true, true, 0)

	window.ShowAll()
	if dialog.Run() != gtk.GTK_RESPONSE_ACCEPT {
		return
	}
	defer killChild()
	changed()
	button.Connect("selection-changed", changed)
	gtk.Main()
}
