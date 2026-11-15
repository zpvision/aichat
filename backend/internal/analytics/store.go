package analytics

import (
	"context"
	"encoding/json"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"time"
)

type Store struct{ DB *pgxpool.Pool }

func (s *Store) Report(ctx context.Context, org string, f Filter) (json.RawMessage, error) {
	tx, e := s.DB.BeginTx(ctx, pgx.TxOptions{AccessMode: pgx.ReadOnly})
	if e != nil {
		return nil, e
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = tx.Rollback(cleanup)
	}()
	// Measured PostgreSQL JIT compilation dominated this bounded aggregate query.
	// Keep the setting local to this short read transaction, never change the server.
	if _, e = tx.Exec(ctx, "SET LOCAL jit=off"); e != nil {
		return nil, e
	}
	// Freshly imported tenants can have stale cardinality estimates. Measurements
	// exposed quadratic nested-loop joins over materialized aggregates. Hash/merge
	// joins keep this report bounded; required indexed LATERAL loops remain possible.
	if _, e = tx.Exec(ctx, "SET LOCAL enable_nestloop=off"); e != nil {
		return nil, e
	}
	var data []byte
	e = tx.QueryRow(ctx, reportSQL, org, f.Start, f.End, f.Previous, f.Zone, f.Site, f.Assistant, f.Page*20, f.Sort).Scan(&data)
	if e != nil {
		return nil, e
	}
	if e = tx.Commit(ctx); e != nil {
		return nil, e
	}
	return json.RawMessage(data), nil
}

// One read statement, one MVCC snapshot, no message contents, no row locks.
const reportSQL = `WITH
c AS NOT MATERIALIZED (
 SELECT c.id,c.organization_id,c.site_id,c.assistant_id,c.created_at,c.assigned_user_id,c.control_mode FROM conversations c WHERE c.organization_id=$1 AND c.kind='website'
 AND ($6='' OR c.site_id=NULLIF($6,'')::uuid) AND ($7='' OR c.assistant_id=NULLIF($7,'')::uuid)
),
facts AS MATERIALIZED (
 SELECT c.*,
 (SELECT min(m.created_at) FROM conversation_messages m WHERE m.organization_id=$1 AND m.conversation_id=c.id AND m.role='user') visitor_at,
 (SELECT l.submitted_at FROM lead_collections l WHERE l.organization_id=$1 AND l.conversation_id=c.id AND l.state='submitted') submitted_at,
 (SELECT min(m.created_at) FROM conversation_messages m WHERE m.organization_id=$1 AND m.conversation_id=c.id AND m.role='operator' AND m.status='completed') reply_at,
 (SELECT min(e.created_at) FROM conversation_events e WHERE e.organization_id=$1 AND e.conversation_id=c.id AND e.kind='human_active') takeover_at
 FROM c WHERE c.created_at>=$4 AND c.created_at<$3
),
cohort AS MATERIALIZED (SELECT facts.*,visitor_at<$3 eligible,submitted_at<$3 converted,COALESCE(LEAST(reply_at,takeover_at)<$3,false) manual FROM facts),
ev AS MATERIALIZED (SELECT e.id,e.conversation_id,e.sequence,e.kind,e.actor_id,e.created_at,c.site_id,c.assistant_id FROM conversation_events e JOIN c ON c.id=e.conversation_id WHERE e.organization_id=$1 AND e.created_at>=$4 AND e.created_at<$3 AND e.kind<>'note'),
ex AS MATERIALIZED (SELECT e.id,e.conversation_id,e.status,e.provider,e.created_at,e.input_tokens,e.output_tokens,e.provider_duration_ns,e.policy_fallback,(e.settings_snapshot<>'{}'::jsonb) prepared FROM ai_executions e JOIN c ON c.id=e.conversation_id WHERE e.organization_id=$1 AND e.created_at>=$4 AND e.created_at<$3),
l AS MATERIALIZED (SELECT l.site_id,l.assistant_id,l.conversation_id,l.source,l.created_at,l.status FROM leads l WHERE l.organization_id=$1 AND l.created_at>=$4 AND l.created_at<$3 AND ($6='' OR l.site_id=NULLIF($6,'')::uuid) AND ($7='' OR l.assistant_id=NULLIF($7,'')::uuid)),
periods AS (SELECT 'current' label,$2::timestamptz a,$3::timestamptz b UNION ALL SELECT 'previous',$4,$2),
kpi AS (
 SELECT p.label,
 (SELECT count(*) FROM cohort WHERE created_at>=p.a AND created_at<p.b) dialogs,
 (SELECT count(*) FROM l WHERE source IN ('website','staff') AND conversation_id IS NOT NULL AND created_at>=p.a AND created_at<p.b) leads,
 (SELECT count(*) FROM cohort WHERE created_at>=p.a AND created_at<p.b AND visitor_at<p.b) eligible,
 (SELECT count(*) FROM cohort WHERE created_at>=p.a AND created_at<p.b AND visitor_at<p.b AND submitted_at<p.b) converted,
 (SELECT count(*) FROM cohort ch WHERE ch.created_at>=p.a AND ch.created_at<p.b
 AND (reply_at IS NULL OR reply_at>=p.b) AND (takeover_at IS NULL OR takeover_at>=p.b)) without_operator,
 (SELECT count(DISTINCT conversation_id) FROM ev WHERE kind IN ('human_requested','human_active') AND created_at>=p.a AND created_at<p.b) handoffs,
 (SELECT avg(provider_duration_ns)/1000000.0 FROM ex WHERE provider='ollama' AND status='completed' AND provider_duration_ns>0 AND created_at>=p.a AND created_at<p.b) generation_ms
 FROM periods p
),
days AS (SELECT generate_series(($2 AT TIME ZONE $5)::date,(($3 AT TIME ZONE $5)::date-1),interval '1 day')::date AS bucket_date),
series AS (SELECT bucket_date,
 (SELECT count(*) FROM cohort WHERE created_at>=$2 AND (created_at AT TIME ZONE $5)::date=bucket_date) dialogs,
 (SELECT count(*) FROM l WHERE source IN ('website','staff') AND conversation_id IS NOT NULL AND created_at>=$2 AND (created_at AT TIME ZONE $5)::date=bucket_date) leads,
 (SELECT count(DISTINCT conversation_id) FROM ev WHERE kind IN ('human_requested','human_active') AND created_at>=$2 AND (created_at AT TIME ZONE $5)::date=bucket_date) handoffs,
 (SELECT count(DISTINCT conversation_id) FROM ex WHERE created_at>=$2 AND (created_at AT TIME ZONE $5)::date=bucket_date AND prepared AND policy_fallback) no_sources,
 (SELECT count(*) FROM ex WHERE created_at>=$2 AND (created_at AT TIME ZONE $5)::date=bucket_date AND status='failed') failed,
 (SELECT count(*) FROM ex WHERE created_at>=$2 AND (created_at AT TIME ZONE $5)::date=bucket_date AND status='canceled') canceled
 FROM days),
groupkeys AS (SELECT site_id,assistant_id FROM cohort WHERE created_at>=$2 UNION SELECT site_id,assistant_id FROM l WHERE source IN ('website','staff') AND conversation_id IS NOT NULL AND created_at>=$2 UNION SELECT ev.site_id,ev.assistant_id FROM ev WHERE ev.kind IN ('human_requested','human_active') AND ev.created_at>=$2),
groups AS (SELECT k.site_id,k.assistant_id,count(ch.id) dialogs,count(ch.id) FILTER(WHERE eligible) eligible,count(ch.id) FILTER(WHERE eligible AND converted) converted,
 (SELECT count(*) FROM l WHERE l.site_id=k.site_id AND l.assistant_id=k.assistant_id AND l.source IN ('website','staff') AND conversation_id IS NOT NULL AND l.created_at>=$2) leads,
 (SELECT count(DISTINCT conversation_id) FROM ev WHERE ev.site_id=k.site_id AND ev.assistant_id=k.assistant_id AND ev.kind IN ('human_requested','human_active') AND ev.created_at>=$2) handoffs
 FROM groupkeys k LEFT JOIN cohort ch ON ch.site_id=k.site_id AND ch.assistant_id=k.assistant_id AND ch.created_at>=$2 GROUP BY k.site_id,k.assistant_id),
ranked AS (SELECT g.*,s.name site,a.name assistant, CASE WHEN eligible>0 THEN converted::numeric/eligible ELSE NULL END conversion FROM groups g LEFT JOIN sites s ON s.organization_id=$1 AND s.id=g.site_id JOIN assistants a ON a.organization_id=$1 AND a.id=g.assistant_id),
issues AS (SELECT e.id,e.conversation_id,c.site_id,c.assistant_id,s.name site,a.name assistant,e.created_at,
 CASE WHEN e.status='failed' THEN 'execution_error' ELSE 'no_sources' END category
 FROM ex e JOIN c ON c.id=e.conversation_id LEFT JOIN sites s ON s.organization_id=$1 AND s.id=c.site_id JOIN assistants a ON a.organization_id=$1 AND a.id=c.assistant_id
 WHERE e.created_at>=$2 AND (e.status='failed' OR (e.prepared AND e.policy_fallback)) ORDER BY e.created_at DESC,e.id DESC),
controls AS (SELECT e.id,e.conversation_id,e.created_at,e.sequence,e.kind,lag(e.kind) OVER(PARTITION BY e.conversation_id ORDER BY e.sequence) prior FROM conversation_events e JOIN c ON c.id=e.conversation_id WHERE e.organization_id=$1 AND e.kind IN ('human_requested','human_active','ai_active','closed') AND e.created_at<$3),
episodes AS (SELECT e.*, (SELECT min(x.sequence) FROM controls x WHERE x.conversation_id=e.conversation_id AND x.sequence>e.sequence AND x.kind IN ('ai_active','closed')) stop_sequence FROM controls e WHERE e.kind IN ('human_requested','human_active') AND COALESCE(prior,'') NOT IN ('human_requested','human_active') AND e.created_at>=$2),
responses AS (SELECT e.id,e.conversation_id,m.author_user_id,extract(epoch FROM(m.created_at-e.created_at))*1000 response_ms FROM episodes e JOIN LATERAL (SELECT author_user_id,created_at FROM conversation_messages m WHERE m.organization_id=$1 AND m.conversation_id=e.conversation_id AND m.role='operator' AND m.status='completed' AND m.change_seq>e.sequence AND (e.stop_sequence IS NULL OR m.change_seq<e.stop_sequence) AND m.created_at<$3 ORDER BY m.sequence LIMIT 1) m ON true WHERE m.created_at>=e.created_at),
team AS (SELECT om.user_id,u.name,
 (SELECT count(DISTINCT conversation_id) FROM ev WHERE kind='human_active' AND actor_id=om.user_id AND created_at>=$2) taken,
 (SELECT count(*) FROM conversation_messages m JOIN c ON c.id=m.conversation_id WHERE m.organization_id=$1 AND m.role='operator' AND m.status='completed' AND m.author_user_id=om.user_id AND m.created_at>=$2 AND m.created_at<$3) replies,
 (SELECT count(DISTINCT conversation_id) FROM ev WHERE kind='closed' AND actor_id=om.user_id AND created_at>=$2) closed,
 (SELECT count(*) FROM c WHERE assigned_user_id=om.user_id AND control_mode<>'closed') assigned_now,
 (SELECT avg(response_ms) FROM responses WHERE author_user_id=om.user_id) response_ms,
 (SELECT count(*) FROM responses WHERE author_user_id=om.user_id) response_samples
 FROM organization_members om JOIN users u ON u.id=om.user_id WHERE om.organization_id=$1)
SELECT jsonb_build_object(
 'period',jsonb_build_object('start',$2::timestamptz,'end',$3::timestamptz,'previous_start',$4::timestamptz,'timezone',$5::text),
 'kpis',(SELECT jsonb_object_agg(label,to_jsonb(kpi)-'label') FROM kpi),
 'timeseries',COALESCE((SELECT jsonb_agg(to_jsonb(series) ORDER BY bucket_date) FROM series),'[]'),
 'handling',jsonb_build_object('without_operator',(SELECT count(*) FROM cohort WHERE created_at>=$2 AND NOT manual),'with_operator',(SELECT count(*) FROM cohort WHERE created_at>=$2 AND manual),'waiting_now',(SELECT count(*) FROM c WHERE control_mode='human_requested'),'human_active_now',(SELECT count(*) FROM c WHERE control_mode='human_active')),
 'executions',COALESCE((SELECT jsonb_agg(x) FROM (SELECT provider,status,count(*) count,sum(input_tokens) input_tokens,sum(output_tokens) output_tokens FROM ex WHERE created_at>=$2 GROUP BY provider,status ORDER BY provider,status)x),'[]'),
 'lead_sources',COALESCE((SELECT jsonb_agg(x) FROM (SELECT source,status,count(*) count FROM l WHERE created_at>=$2 GROUP BY source,status ORDER BY source,status)x),'[]'),
 'sites',COALESCE((SELECT jsonb_agg(x) FROM (SELECT * FROM ranked ORDER BY CASE WHEN $9='dialogs' THEN dialogs WHEN $9='leads' THEN leads WHEN $9='handoffs' THEN handoffs ELSE conversion END DESC NULLS LAST,site_id,assistant_id LIMIT 20 OFFSET $8)x),'[]'),
 'sites_total',(SELECT count(*) FROM ranked),
 'insights',jsonb_build_object('top_site',(SELECT to_jsonb(x) FROM (SELECT site_id,max(site) site,sum(leads) leads FROM ranked GROUP BY site_id HAVING sum(leads)>0 ORDER BY sum(leads) DESC,site_id LIMIT 1)x),'top_assistant',(SELECT to_jsonb(x) FROM (SELECT assistant_id,max(assistant) assistant,sum(handoffs) handoffs FROM ranked GROUP BY assistant_id HAVING sum(handoffs)>0 ORDER BY sum(handoffs) DESC,assistant_id LIMIT 1)x)),
 'attention',COALESCE((SELECT jsonb_agg(x) FROM (SELECT * FROM issues LIMIT 20 OFFSET $8)x),'[]'),
 'attention_total',(SELECT count(*) FROM issues),
 'quality',jsonb_build_object('no_sources',(SELECT count(DISTINCT conversation_id) FROM ex WHERE created_at>=$2 AND prepared AND policy_fallback),'failed',(SELECT count(*) FROM ex WHERE created_at>=$2 AND status='failed'),'canceled',(SELECT count(*) FROM ex WHERE created_at>=$2 AND status='canceled')),
 'operators',COALESCE((SELECT jsonb_agg(x) FROM (SELECT * FROM team ORDER BY replies DESC,user_id LIMIT 20 OFFSET $8)x),'[]'),
 'operators_total',(SELECT count(*) FROM team)
)`
