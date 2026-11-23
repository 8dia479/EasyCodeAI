package agent

import (
 "context"
 "fmt"
 "io"
 "strings"
 "github.com/8dia479/EasyCodeAI/internal/nim"
 "github.com/8dia479/EasyCodeAI/internal/tools"
 "github.com/8dia479/EasyCodeAI/internal/workspace"
)
type Options struct{MaxRounds int}
type Agent struct{client *nim.Client;ws *workspace.Workspace;opts Options;tools *tools.Registry}
func New(client *nim.Client,ws *workspace.Workspace,opts Options)*Agent{if opts.MaxRounds<=0{opts.MaxRounds=20};return &Agent{client,ws,opts,tools.New(ws)}}
func(a *Agent)Run(ctx context.Context,task string,out io.Writer)error{cp:=Checkpoint{Task:task,Phase:"starting"};messages:=[]nim.Message{{Role:"system",Content:systemPrompt(a.ws.Root)},{Role:"user",Content:task}};specs:=a.tools.Specs();for round:=1;round<=a.opts.MaxRounds;round++{cp.Phase=fmt.Sprintf("model_round_%d",round);cp.History=append(cp.History,cp.Phase);if e:=saveCheckpoint(cp);e!=nil{return e};fmt.Fprintf(out,"\n\x1b[36m[EasyCodeAI · round %d/%d]\x1b[0m\n",round,a.opts.MaxRounds);reply,e:=a.client.Complete(ctx,messages,specs,func(s string){_,_=io.WriteString(out,s)});if e!=nil{cp.Phase="interrupted";cp.History=append(cp.History,e.Error());_=saveCheckpoint(cp);return e};messages=append(messages,reply);if len(reply.ToolCalls)==0{if strings.Contains(reply.Content,"<easycode-done>"){cp.Phase="completed";cp.History=append(cp.History,"completed");return saveCheckpoint(cp)};messages=append(messages,nim.Message{Role:"user",Content:"Continue the task. Use available tools to inspect, edit, and verify. Do not claim completion until checks pass. Finish with <easycode-done> only when complete."});continue};for _,call:=range reply.ToolCalls{fmt.Fprintf(out,"\n\x1b[33m[tool] %s\x1b[0m\n",call.Function.Name);result,te:=a.tools.Execute(ctx,call.Function.Name,call.Function.Arguments);if te!=nil{result="TOOL ERROR: "+te.Error()};cp.History=append(cp.History,"tool "+call.Function.Name+": "+truncate(result,250));messages=append(messages,nim.Message{Role:"tool",ToolCallID:call.ID,Content:result});fmt.Fprintln(out,result)};if e:=saveCheckpoint(cp);e!=nil{return e}};cp.Phase="round_limit";_=saveCheckpoint(cp);return fmt.Errorf("reached %d-round safety limit; checkpoint saved",a.opts.MaxRounds)}
func truncate(s string,n int)string{if len(s)>n{return s[:n]+"..."};return s}
func(a *Agent)ShowCheckpoint(out io.Writer)error{cp,e:=loadCheckpoint();if e!=nil{return fmt.Errorf("no saved checkpoint: %w",e)};fmt.Fprintf(out,"Task: %s\nPhase: %s\nUpdated: %s\n",cp.Task,cp.Phase,cp.UpdatedAt.Format("2006-01-02 15:04:05 MST"));for _,v:=range cp.History{fmt.Fprintln(out," -",v)};return nil}
func systemPrompt(root string)string{return "You are EasyCodeAI, an autonomous coding agent in Termux. Workspace: "+root+". Inspect before editing. Use tools for filesystem operations and checks. Never claim a file changed unless write_file succeeded or tests passed unless run_checks succeeded. Stay within workspace; avoid overwriting unrelated files. Work iteratively: inspect, plan, edit, test, diagnose, fix, retest. Finish only when complete and verified, ending with <easycode-done>."}
