package tools

import (
 "bytes"
 "context"
 "encoding/json"
 "fmt"
 "os"
 "os/exec"
 "path/filepath"
 "strings"
 "sync"
 "time"
 "github.com/8dia479/EasyCodeAI/internal/nim"
 "github.com/8dia479/EasyCodeAI/internal/workspace"
)
type Registry struct{ws *workspace.Workspace}
func New(ws *workspace.Workspace)*Registry{return &Registry{ws}}
func str(desc string)map[string]any{return map[string]any{"type":"string","description":desc}}
func schema(props map[string]any,required ...string)map[string]any{return map[string]any{"type":"object","properties":props,"required":required,"additionalProperties":false}}
func(r *Registry)Specs()[]nim.ToolSpec{return []nim.ToolSpec{
 {Type:"function",Function:nim.ToolFunctionSpec{Name:"list_files",Description:"List workspace files. Use . for root.",Parameters:schema(map[string]any{"path":str("relative directory")},"path")}},
 {Type:"function",Function:nim.ToolFunctionSpec{Name:"read_file",Description:"Read a text file inside workspace.",Parameters:schema(map[string]any{"path":str("relative file path")},"path")}},
 {Type:"function",Function:nim.ToolFunctionSpec{Name:"write_file",Description:"Create or replace a text file inside workspace.",Parameters:schema(map[string]any{"path":str("relative path"),"content":str("complete file content")},"path","content")}},
 {Type:"function",Function:nim.ToolFunctionSpec{Name:"run_checks",Description:"Run go test ./..., go vet ./..., and go build ./.... No arbitrary commands.",Parameters:schema(map[string]any{})}},
}}
func(r *Registry)Execute(ctx context.Context,name,args string)(string,error){var in map[string]any;if e:=json.Unmarshal([]byte(args),&in);e!=nil{return "",e};switch name{
 case "list_files":p,_:=in["path"].(string);if p==""||p=="."{p="."};dir:=r.ws.Root;if p!="."{var e error;dir,e=r.ws.Resolve(p);if e!=nil{return "",e}};entries,e:=os.ReadDir(dir);if e!=nil{return "",e};var out []string;for _,v:=range entries{k:="file";if v.IsDir(){k="dir"};out=append(out,k+" "+v.Name())};return strings.Join(out,"\n"),nil
 case "read_file":p,_:=in["path"].(string);v,e:=r.ws.ReadFile(p);if e!=nil{return "",e};if len(v)>100000{v=v[:100000]+"\n[truncated]"};return v,nil
 case "write_file":p,_:=in["path"].(string);v,_:=in["content"].(string);if e:=r.ws.WriteFile(p,v);e!=nil{return "",e};return "Wrote "+filepath.Clean(p),nil
 case "run_checks":
        var out strings.Builder
        for _, args := range [][]string{{"test", "./..."}, {"vet", "./..."}, {"build", "./..."}} {
            checkCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
            cmd := exec.CommandContext(checkCtx, "go", args...)
            cmd.Dir = r.ws.Root
            output := &limitedBuffer{max: 1 << 20}
            cmd.Stdout = output
            cmd.Stderr = output
            err := cmd.Run()
            timedOut := checkCtx.Err() == context.DeadlineExceeded
            cancel()
            fmt.Fprintf(&out, "$ go %s\\n%s\\n", strings.Join(args, " "), output.String())
            if err != nil {
                if timedOut {
                    return out.String(), fmt.Errorf("go %s timed out after 5 minutes", strings.Join(args, " "))
                }
                return out.String(), fmt.Errorf("go %s: %w", strings.Join(args, " "), err)
            }
        }
        return out.String(), nil
 default:return "",fmt.Errorf("unknown tool: %s",name)
}}


type limitedBuffer struct {
	mu        sync.Mutex
	buf       bytes.Buffer
	max       int
	truncated bool
}

func (b *limitedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	remaining := b.max - b.buf.Len()
	if remaining > 0 {
		n := len(p)
		if n > remaining {
			n = remaining
		}
		_, _ = b.buf.Write(p[:n])
	}
	if len(p) > remaining {
		b.truncated = true
	}
	return len(p), nil
}

func (b *limitedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	result := b.buf.String()
	if b.truncated {
		result += "\\n[output truncated at 1 MiB]"
	}
	return result
}
