package main
import (
  "context"
  "fmt"
  "os"
  "path/filepath"
  "github.com/kilrkrow/ollama-mgr/internal/catalog"
)
func main() {
  c := catalog.New(filepath.Join(os.TempDir(),"om-pop"), 0)
  all, err := c.Popular(context.Background())
  fmt.Println("err", err, "n", len(all))
  if len(all) > 0 {
    for i := 0; i < 5 && i < len(all); i++ {
      fmt.Printf("%d %s pulls=%s\n", all[i].Rank, all[i].Name, all[i].Pulls)
    }
  }
  page, total := catalog.PopularPage(all, 25, 2, 10)
  fmt.Println("page2 of top25", len(page), "total", total)
  if len(page) > 0 { fmt.Println("first", page[0].Rank, page[0].Name) }
}
