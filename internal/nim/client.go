package nim

import (
 "bufio"
 "bytes"
 "context"
 "encoding/json"
 "fmt"
 "io"
 "net/http"
 "strings"
 "time"
)

type ToolCall struct { ID string `json:"id"`; Type string `json:"type"`; Function ToolFunction `json:"function"` }
type ToolFunction struct { Name string `json:"name"`; Arguments string `json:"arguments"` }
type Message struct { Role string `json:"role"`; Content string `json:"content"`; ToolCalls []ToolCall `json:"tool_calls,omitempty"`; ToolCallID string `json:"tool_call_id,omitempty"` }
type ToolSpec struct { Type string `json:"type"`; Function ToolFunctionSpec `json:"function"` }
type ToolFunctionSpec struct { Name string `json:"name"`; Description string `json:"description"`; Parameters any `json:"parameters"` }
type Client struct { baseURL, apiKey, model string; http *http.Client }
func NewClient(baseURL, apiKey, model string)*Client{return &Client{strings.TrimRight(baseURL,"/"),apiKey,model,&http.Client{Timeout:0,Transport:&http.Transport{ResponseHeaderTimeout:90*time.Second}}}}
type request struct { Model string `json:"model"`; Messages []Message `json:"messages"`; Stream bool `json:"stream"`; Temperature float64 `json:"temperature,omitempty"`; MaxTokens int `json:"max_tokens,omitempty"`; Tools []ToolSpec `json:"tools,omitempty"`; ToolChoice string `json:"tool_choice,omitempty"` }
type response struct { Choices []struct{Message Message `json:"message"`} `json:"choices"` }
type chunk struct { Choices []struct{Delta struct{Content string `json:"content"`; ToolCalls []struct{Index int `json:"index"`; ID string `json:"id"`; Type string `json:"type"`; Function struct{Name string `json:"name"`; Arguments string `json:"arguments"`} `json:"function"`} `json:"tool_calls"`} `json:"delta"`} `json:"choices"` }
func(c *Client)Complete(ctx context.Context,messages []Message,tools []ToolSpec,onDelta func(string))(Message,error){
 b,e:=json.Marshal(request{c.model,messages,true,0.2,8192,tools,"auto"});if e!=nil{return Message{},e}
 req,e:=http.NewRequestWithContext(ctx,http.MethodPost,c.baseURL+"/chat/completions",bytes.NewReader(b));if e!=nil{return Message{},e};req.Header.Set("Authorization","Bearer "+c.apiKey);req.Header.Set("Content-Type","application/json");req.Header.Set("Accept","text/event-stream")
 resp,e:=c.http.Do(req);if e!=nil{return Message{},fmt.Errorf("NIM request failed: %w",e)};defer resp.Body.Close();if resp.StatusCode<200||resp.StatusCode>=300{data,_:=io.ReadAll(io.LimitReader(resp.Body,1<<20));return Message{},fmt.Errorf("NIM HTTP %d: %s",resp.StatusCode,strings.TrimSpace(string(data)))}
 if !strings.Contains(resp.Header.Get("Content-Type"),"text/event-stream"){var v response;if e=json.NewDecoder(resp.Body).Decode(&v);e!=nil{return Message{},e};if len(v.Choices)==0{return Message{},fmt.Errorf("empty NIM response")};return v.Choices[0].Message,nil}
 var out Message;calls:=map[int]*ToolCall{};sc:=bufio.NewScanner(resp.Body);sc.Buffer(make([]byte,65536),4*1024*1024)
 for sc.Scan(){line:=strings.TrimSpace(sc.Text());if !strings.HasPrefix(line,"data:"){continue};p:=strings.TrimSpace(strings.TrimPrefix(line,"data:"));if p=="[DONE]"{break};var v chunk;if e=json.Unmarshal([]byte(p),&v);e!=nil{return out,e};if len(v.Choices)==0{continue};d:=v.Choices[0].Delta;if d.Content!=""{out.Content+=d.Content;if onDelta!=nil{onDelta(d.Content)}};for _,part:=range d.ToolCalls{tc:=calls[part.Index];if tc==nil{tc=&ToolCall{Type:"function"};calls[part.Index]=tc};if part.ID!=""{tc.ID=part.ID};if part.Type!=""{tc.Type=part.Type};tc.Function.Name+=part.Function.Name;tc.Function.Arguments+=part.Function.Arguments}}
 if e=sc.Err();e!=nil{return out,e};for i:=0;i<len(calls);i++{if tc:=calls[i];tc!=nil{out.ToolCalls=append(out.ToolCalls,*tc)}};out.Role="assistant";if out.Content==""&&len(out.ToolCalls)==0{return out,fmt.Errorf("empty NIM stream")};return out,nil
}
