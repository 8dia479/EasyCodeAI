package tools

import (
 "context"
 "encoding/json"
 "fmt"
 "os"
 "os/exec"
 "path/filepath"
 "strings"
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
 case "run_checks":var out strings.Builder;for _,args:=range [][]string{{"test","./..."},{"vet","./..."},{"build","./..."}}{cmd:=exec.CommandContext(ctx,"go",args...);cmd.Dir=r.ws.Root;b,e:=cmd.CombinedOutput();fmt.Fprintf(&out,"$ go %s\n%s\n",strings.Join(args," "),b);if e!=nil{return out.String(),e}};return out.String(),nil
 default:return "",fmt.Errorf("unknown tool: %s",name)
}}
