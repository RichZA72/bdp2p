package fs

import "os"

func DeleteFile(path string) error {
    return os.RemoveAll(path)
}
