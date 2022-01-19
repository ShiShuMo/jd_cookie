package jd_cookie

import (
	"fmt"
	"net/url"

	"github.com/beego/beego/v2/core/logs"
	"github.com/cdle/sillyGirl/core"
	"github.com/cdle/sillyGirl/develop/qinglong"
)

var dzs = "	——来自大芝士"

func initRongQi() {
	core.AddCommand("", []core.Function{
		{
			Rules: []string{"迁移"},
			Admin: true,
			// Cron:  "*/5 * * * *",
			Handle: func(s core.Sender) interface{} {
				if it := s.GetImType(); it != "terminal" && it != "tg" && it != "fake" {
					return "可能会产生大量消息，请在终端或tg进行操作。"
				}
				//容器内去重
				var memvs = map[*qinglong.QingLong][]qinglong.Env{} //分组记录ck
				var aggregated = []*qinglong.QingLong{}
				var uaggregated = []*qinglong.QingLong{}
				for _, ql := range qinglong.GetQLS() {
					if ql.AggregatedMode {
						aggregated = append(aggregated, ql)
					} else {
						uaggregated = append(uaggregated, ql)
					}
					envs, err := qinglong.GetEnvs(ql, "JD_COOKIE")
					if err == nil {
						var mc = map[string]bool{}
						nn := []qinglong.Env{}
						for _, env := range envs {
							if env.Status == 0 {
								env.PtPin = core.FetchCookieValue(env.Value, "pt_pin")
								if env.PtPin == "" {
									continue
								}
								name, _ = url.QueryUnescape(env.PtPin)
								if _, ok := mc[env.PtPin]; ok {
									if _, err := qinglong.Req(ql, qinglong.PUT, qinglong.ENVS, "/disable", []byte(`["`+env.ID+`"]`)); err == nil {
										s.Reply(fmt.Sprintf("发现到重复变量，已隐藏(%s)。%s", name, ql.GetTail()))
									}
									env.Remarks = "重复变量。"
									qinglong.UdpEnv(ql, env)
								} else {
									mc[env.PtPin] = true
									nn = append(nn, env)
								}
							}
						}
						memvs[ql] = nn
					}
				}
				//容器间去重
				var eql = map[string]*qinglong.QingLong{}
				for ql, envs := range memvs {
					if ql.AggregatedMode {
						continue
					}
					nn := []qinglong.Env{}
					for _, env := range envs {
						name, _ = url.QueryUnescape(env.PtPin)
						if _, ok := eql[env.PtPin]; ok {
							if _, err := qinglong.Req(ql, qinglong.PUT, qinglong.ENVS, "/disable", []byte(`["`+env.ID+`"]`)); err == nil {
								s.Reply(fmt.Sprintf("在容器(%s)发现重复变量，已隐藏(%s)。%s", ql.GetName(), name, dzs))
							}
							env.Remarks = "重复变量。"
							qinglong.UdpEnv(ql, env)
						} else {
							eql[env.PtPin] = ql
							nn = append(nn, env)
						}
					}
					memvs[ql] = nn
				}
				//聚合
				for _, aql := range aggregated {
					toapp := []qinglong.Env{}
					for ql, envs := range memvs {
						toapp_ := []qinglong.Env{}
						if ql == aql {
							continue
						}
						for _, env := range envs {
							if !envContain(append(memvs[aql], toapp...), env) {
								toapp = append(toapp, env)
								toapp_ = append(toapp_, env)
							}
						}
						if len(toapp_) > 0 {
							memvs[aql] = append(memvs[aql], toapp_...)
							if err := qinglong.AddEnv(aql, toapp_...); err != nil {
								s.Reply(fmt.Sprintf("失败从容器(%s)转移%d个变量到聚合容器(%s)：%v", ql.GetName(), len(toapp_), aql.GetName(), err))
							} else {
								s.Reply(fmt.Sprintf("成功从容器(%s)转移%d个变量到聚合容器(%s)。%s", ql.GetName(), len(toapp_), aql.GetName(), dzs))
							}
						}
					}
				}
				ts := len(uaggregated)
				es := []int{} //变量数集合
				ess := 0
				ws := []int{} //权重集合
				wss := 0
				rs := []int{} //结果集合

				for i := range uaggregated {
					es = append(es, len(memvs[uaggregated[i]]))
					ws = append(ws, uaggregated[i].GetWeight())
					wss += ws[i]
					ess += es[i]

				}

				for i := range uaggregated {
					if i != ts-1 {
						rs = append(rs, ess*ws[i]/wss)
						continue
					}
					v := 0
					for _, r := range rs {
						v += r
					}
					rs = append(rs, ess-v)
				}
				logs.Info("ts", ts)
				logs.Info("es", es)
				logs.Info("ess", ess)
				logs.Info("ws", ws)
				logs.Info("wss", wss)
				logs.Info("rs", rs)
				torem := map[*qinglong.QingLong][]qinglong.Env{}
				for i := range uaggregated {
					if rs[i] > es[i] {
						torem[uaggregated[i]] = memvs[uaggregated[i]][:es[i]]
					}
				}
				for i, iql := range uaggregated {
					if rs[i] < es[i] {
						need := es[i] - rs[i]
						for oql := range torem {
							tr := []qinglong.Env{}
							if l := len(torem[oql]); l > need {
								tr = torem[oql][l-need:]
								need = 0
							} else if l > 0 && l < need {
								tr = torem[oql]
								need -= l
							}
							if err := qinglong.AddEnv(iql, tr...); err != nil {
								s.Reply(fmt.Sprintf("失败从容器(%s)转移%d个变量到容器(%s)：%v", oql.GetName(), len(tr), err, iql.GetName()))
							} else {
								if err := qinglong.RemEnv(oql, tr...); err != nil {
									s.Reply(fmt.Sprintf("删除%d个变量失败：%v%s", len(tr), err, oql.GetTail()))
								} else {
									s.Reply(fmt.Sprintf("成功从容器(%s)转移%d个变量到容器(%s)。%s", oql.GetName(), len(tr), iql.GetName(), dzs))
								}
							}
						}
					}
				}
				return "迁移任务结束。"
			},
		},
	})
}

func envContain(ay []qinglong.Env, e qinglong.Env) bool {
	for _, v := range ay {
		if v.PtPin == e.PtPin {
			return true
		}
	}
	return false
}
