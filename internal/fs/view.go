package fs

import (
    "io/fs"
    "path/filepath"
)

type FileInfo struct {
    Name string
    Path string
    IsDir bool
}

func ListFiles(root string) ([]FileInfo, error) {
    var files []FileInfo
    err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
        if err != nil {
            return err
        }
        if path == root {
            return nil
        }
        files = append(files, FileInfo{
            Name: d.Name(),
            Path: path,
            IsDir: d.IsDir(),
        })
        return nil
    })
    return files, err
}
