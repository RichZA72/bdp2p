package gui

import (
    "encoding/json"
    "fmt"
    "image/color"
    "net"
    "os"
    "os/exec"
    "path/filepath"
    "p2pfs/internal/fs"
    "runtime"
    "strings"
    "time"

    "fyne.io/fyne/v2"
    "fyne.io/fyne/v2/app"
    "fyne.io/fyne/v2/canvas"
    "fyne.io/fyne/v2/container"
    "fyne.io/fyne/v2/layout"
    "fyne.io/fyne/v2/widget"
)

type Peer struct {
    ID   int    `json:"id"`
    IP   string `json:"ip"`
    Port string `json:"port"`
}

var grid *fyne.Container
var selectedFile *fs.FileInfo
var selectedHighlight *canvas.Rectangle
var peers []Peer
var localID int = 1 // esta máquina es Maq1

func Run() {
    a := app.New()
    w := a.NewWindow("P2P File Explorer")
    w.Resize(fyne.NewSize(1000, 700))

    loadPeers()

    eliminarBtn := widget.NewButton("Eliminar", func() {
        if selectedFile == nil {
            fmt.Println("No hay archivo seleccionado")
            return
        }
        err := fs.DeleteFile(selectedFile.Path)
        if err != nil {
            fmt.Println("Error al eliminar:", err)
        } else {
            fmt.Println("Archivo eliminado:", selectedFile.Name)
            selectedFile = nil
            selectedHighlight = nil
            refreshFiles()
        }
    })

    transferirBtn := widget.NewButton("Transferir", func() {
        if selectedFile == nil {
            fmt.Println("No hay archivo seleccionado")
            return
        }
        fmt.Printf("Simulando transferencia de '%s' al nodo destino...\n", selectedFile.Name)
    })

    actualizarBtn := widget.NewButton("Actualizar", func() {
        refreshFiles()
    })

    maquinaEntry := widget.NewEntry()
    maquinaEntry.SetPlaceHolder("Máquina:")

    controlBar := container.NewHBox(
        eliminarBtn,
        transferirBtn,
        actualizarBtn,
        maquinaEntry,
    )

    grid = container.NewGridWithColumns(2)
    refreshFiles()

    content := container.NewBorder(controlBar, nil, nil, nil, grid)
    w.SetContent(content)
    w.ShowAndRun()
}

func refreshFiles() {
    os.MkdirAll("./shared", os.ModePerm)
    localFiles, _ := fs.ListFiles("./shared")

    panels := []fyne.CanvasObject{}
    for _, p := range peers {
        files := []fs.FileInfo{}
        if p.ID == localID {
            files = localFiles
        }
        panels = append(panels, renderTable(p, files))
    }

    grid.Objects = panels
    grid.Refresh()
}

func renderTable(peer Peer, files []fs.FileInfo) *fyne.Container {
    status := "🔴"
    if peer.ID == localID || isPeerOnline(peer.IP, peer.Port) {
        status = "🟢"
    }

    titleText := fmt.Sprintf("Maq%d (%s:%s) %s", peer.ID, peer.IP, peer.Port, status)
    title := widget.NewLabelWithStyle(titleText, fyne.TextAlignCenter, fyne.TextStyle{Bold: true})
    titleBox := container.New(layout.NewCenterLayout(), title)

    header := container.NewHBox(
        widget.NewLabelWithStyle("Nombre", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
        layout.NewSpacer(),
        widget.NewLabelWithStyle("Fecha de modificación", fyne.TextAlignTrailing, fyne.TextStyle{Bold: true}),
    )

    items := []fyne.CanvasObject{titleBox, header}

    for _, f := range files {
        modTime := ""
        if info, err := os.Stat(f.Path); err == nil {
            modTime = info.ModTime().Format("02/01/2006 03:04 p. m.")
        }

        icon := fileIcon(f)
        name := widget.NewLabel(fmt.Sprintf("%s %s", icon, f.Name))
        date := widget.NewLabel(modTime)
        row := container.NewHBox(name, layout.NewSpacer(), date)

        file := f
        bg := canvas.NewRectangle(color.NRGBA{R: 0, G: 200, B: 255, A: 100})
        bg.Hide()

        fileBtn := widget.NewButton("", func() {
            if selectedFile != nil && selectedFile.Path == file.Path {
                openFile(file.Path)
                return
            }
            if selectedHighlight != nil {
                selectedHighlight.Hide()
                canvas.Refresh(selectedHighlight)
            }
            selectedFile = &file
            selectedHighlight = bg
            bg.Show()
            canvas.Refresh(bg)
            fmt.Println("Archivo seleccionado:", file.Name)
        })
        fileBtn.Importance = widget.LowImportance
        fileBtn.Resize(fyne.NewSize(0, 0))

        item := container.NewMax(bg, row, fileBtn)
        items = append(items, item)
    }

    content := container.NewVBox(items...)

    // Bordes en todos los lados
    lineColor := color.NRGBA{R: 180, G: 180, B: 180, A: 255}
    top := canvas.NewLine(lineColor)
    bottom := canvas.NewLine(lineColor)
    left := canvas.NewLine(lineColor)
    right := canvas.NewLine(lineColor)
    top.StrokeWidth = 2
    bottom.StrokeWidth = 2
    left.StrokeWidth = 2
    right.StrokeWidth = 2

    bordered := container.NewBorder(top, bottom, left, right, content)
    return bordered
}

func fileIcon(f fs.FileInfo) string {
    if f.IsDir {
        return "📁"
    }

    ext := strings.ToLower(filepath.Ext(f.Name))
    switch ext {
    case ".txt", ".pdf", ".docx":
        return "📄"
    case ".jpg", ".jpeg", ".png":
        return "🖼️"
    case ".mp3", ".wav":
        return "🎵"
    case ".mp4", ".avi", ".mkv":
        return "🎥"
    default:
        return "❓"
    }
}

func openFile(path string) {
    var cmd *exec.Cmd
    switch runtime.GOOS {
    case "linux":
        cmd = exec.Command("xdg-open", path)
    case "windows":
        cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", path)
    case "darwin":
        cmd = exec.Command("open", path)
    }

    if err := cmd.Start(); err != nil {
        fmt.Println("No se pudo abrir el archivo:", err)
    } else {
        fmt.Println("Abriendo archivo:", path)
    }
}

func loadPeers() {
    data, err := os.ReadFile("config/peers.json")
    if err != nil {
        fmt.Println("No se pudo leer peers.json:", err)
        return
    }
    json.Unmarshal(data, &peers)
}

func isPeerOnline(ip, port string) bool {
    conn, err := net.DialTimeout("tcp", net.JoinHostPort(ip, port), 500*time.Millisecond)
    if err != nil {
        return false
    }
    conn.Close()
    return true
}
