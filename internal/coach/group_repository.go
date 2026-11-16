package coach

import (
	"context"
	"errors"
	"fmt"

	"github.com/Masterminds/squirrel"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nekogravitycat/court-booking-backend/internal/pkg/pagination"
	"github.com/nekogravitycat/court-booking-backend/internal/pkg/request"
)

// InvitationFilter selects invitations by group and/or invitee.
type InvitationFilter struct {
	GroupID   string
	InviteeID string
	Status    string
	Page      int
	PageSize  int
}

// GroupRepository persists coach groups, members and invitations.
//
// Operations that read-then-write membership lock the group row FOR UPDATE, so
// the "at least one leader" rule cannot be broken by concurrent requests. The
// "one group per coach" rule is enforced by the unique constraint on
// coach_group_members.user_id.
type GroupRepository interface {
	CreateGroup(ctx context.Context, name string, allowPrivate bool, creatorID string) (string, error)
	GetGroup(ctx context.Context, id string) (*Group, error)
	// GetGroupByMember returns the group the user belongs to, or ErrNotInGroup.
	GetGroupByMember(ctx context.Context, userID string) (*Group, error)
	ListGroups(ctx context.Context, filter GroupFilter) ([]*Group, int, error)
	UpdateGroup(ctx context.Context, id string, name *string, allowPrivate *bool) error
	// DisbandGroup deactivates the group, frees its members, cancels open
	// invitations, and returns the IDs of the former members.
	DisbandGroup(ctx context.Context, id string) ([]string, error)
	GetMember(ctx context.Context, groupID, userID string) (*Member, error)
	UpdateMember(ctx context.Context, groupID, userID string, u MemberUpdate) (*Member, error)
	// RemoveMember removes a member. It reports whether the group was disbanded
	// because the last member left.
	RemoveMember(ctx context.Context, groupID, userID string) (disbanded bool, err error)
	CreateInvitation(ctx context.Context, groupID, inviteeID, role, invitedBy string) (*Invitation, error)
	GetInvitation(ctx context.Context, id string) (*Invitation, error)
	ListInvitations(ctx context.Context, filter InvitationFilter) ([]*Invitation, int, error)
	CancelInvitation(ctx context.Context, groupID, id string) error
	// RespondInvitation accepts or declines a pending invitation for the invitee.
	RespondInvitation(ctx context.Context, id, inviteeID string, accept bool) (*Invitation, error)
	// LeaderIDs returns the user IDs of the group's leaders.
	LeaderIDs(ctx context.Context, groupID string) ([]string, error)
}

type pgxGroupRepository struct {
	pool *pgxpool.Pool
}

func NewPgxGroupRepository(pool *pgxpool.Pool) GroupRepository {
	return &pgxGroupRepository{pool: pool}
}

const memberSelect = `SELECT m.group_id, m.user_id, COALESCE(u.display_name, u.username), u.avatar, m.role, m.status, m.joined_at
	FROM public.coach_group_members m JOIN public.users u ON u.id = m.user_id`

func scanMember(row pgx.Row) (*Member, error) {
	var m Member
	if err := row.Scan(&m.GroupID, &m.UserID, &m.DisplayName, &m.Avatar, &m.Role, &m.Status, &m.JoinedAt); err != nil {
		return nil, err
	}
	return &m, nil
}

func (r *pgxGroupRepository) loadMembers(ctx context.Context, q querier, groups []*Group) error {
	if len(groups) == 0 {
		return nil
	}
	ids := make([]string, len(groups))
	byID := make(map[string]*Group, len(groups))
	for i, g := range groups {
		ids[i] = g.ID
		byID[g.ID] = g
		g.Members = []*Member{}
	}
	rows, err := q.Query(ctx, memberSelect+` WHERE m.group_id = ANY($1::uuid[])
		ORDER BY (m.role = 'leader') DESC, m.joined_at, m.user_id`, ids)
	if err != nil {
		return fmt.Errorf("load group members failed: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		m, err := scanMember(rows)
		if err != nil {
			return fmt.Errorf("scan member failed: %w", err)
		}
		byID[m.GroupID].Members = append(byID[m.GroupID].Members, m)
	}
	return rows.Err()
}

func (r *pgxGroupRepository) CreateGroup(ctx context.Context, name string, allowPrivate bool, creatorID string) (string, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer tx.Rollback(ctx)

	var id string
	if err := tx.QueryRow(ctx, `INSERT INTO public.coach_groups (name, allow_private_courses, created_by)
		VALUES ($1, $2, $3) RETURNING id`, name, allowPrivate, creatorID).Scan(&id); err != nil {
		return "", fmt.Errorf("create coach group failed: %w", err)
	}
	if _, err := tx.Exec(ctx,
		"INSERT INTO public.coach_group_members (group_id, user_id, role) VALUES ($1, $2, 'leader')", id, creatorID); err != nil {
		if isUniqueViolation(err) {
			return "", ErrAlreadyInGroup
		}
		return "", fmt.Errorf("add group leader failed: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return "", err
	}
	return id, nil
}

const groupSelect = `SELECT g.id, g.name, g.allow_private_courses, g.is_active, g.created_at, g.updated_at,
	(SELECT count(*) FROM public.coach_group_members m WHERE m.group_id = g.id)
	FROM public.coach_groups g`

func scanGroup(row pgx.Row, extra ...any) (*Group, error) {
	var g Group
	dest := append([]any{&g.ID, &g.Name, &g.AllowPrivateCourses, &g.IsActive, &g.CreatedAt, &g.UpdatedAt, &g.MemberCount}, extra...)
	if err := row.Scan(dest...); err != nil {
		return nil, err
	}
	return &g, nil
}

func (r *pgxGroupRepository) GetGroup(ctx context.Context, id string) (*Group, error) {
	g, err := scanGroup(r.pool.QueryRow(ctx, groupSelect+" WHERE g.id = $1 AND g.is_active", id))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrGroupNotFound
		}
		return nil, fmt.Errorf("get coach group failed: %w", err)
	}
	if err := r.loadMembers(ctx, r.pool, []*Group{g}); err != nil {
		return nil, err
	}
	return g, nil
}

func (r *pgxGroupRepository) GetGroupByMember(ctx context.Context, userID string) (*Group, error) {
	var id string
	err := r.pool.QueryRow(ctx, `SELECT m.group_id FROM public.coach_group_members m
		JOIN public.coach_groups g ON g.id = m.group_id AND g.is_active WHERE m.user_id = $1`, userID).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotInGroup
	}
	if err != nil {
		return nil, fmt.Errorf("get member group failed: %w", err)
	}
	return r.GetGroup(ctx, id)
}

func (r *pgxGroupRepository) ListGroups(ctx context.Context, f GroupFilter) ([]*Group, int, error) {
	psql := squirrel.StatementBuilder.PlaceholderFormat(squirrel.Dollar)
	q := psql.Select("g.id", "g.name", "g.allow_private_courses", "g.is_active", "g.created_at", "g.updated_at",
		"(SELECT count(*) FROM public.coach_group_members m WHERE m.group_id = g.id)",
		"count(*) OVER() AS total_count").
		From("public.coach_groups g").Where("g.is_active").
		OrderBy("g.created_at DESC", "g.id")
	if f.Keyword != "" {
		q = q.Where("g.name ILIKE ?", "%"+request.EscapeLike(f.Keyword)+"%")
	}
	items, total, err := pagination.Collect(ctx, r.pool, q, f.Page, f.PageSize, "coach group", func(rows pgx.Rows, total *int) (*Group, error) {
		g, err := scanGroup(rows, total)
		if err != nil {
			return nil, fmt.Errorf("scan coach group failed: %w", err)
		}
		return g, nil
	})
	if err != nil {
		return nil, 0, err
	}
	if err := r.loadMembers(ctx, r.pool, items); err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

func (r *pgxGroupRepository) UpdateGroup(ctx context.Context, id string, name *string, allowPrivate *bool) error {
	set := squirrel.Update("public.coach_groups").PlaceholderFormat(squirrel.Dollar).Set("updated_at", squirrel.Expr("now()"))
	if name != nil {
		set = set.Set("name", *name)
	}
	if allowPrivate != nil {
		set = set.Set("allow_private_courses", *allowPrivate)
	}
	sql, args, err := set.Where(squirrel.Eq{"id": id, "is_active": true}).ToSql()
	if err != nil {
		return fmt.Errorf("build group update failed: %w", err)
	}
	ct, err := r.pool.Exec(ctx, sql, args...)
	if err != nil {
		return fmt.Errorf("update coach group failed: %w", err)
	}
	if ct.RowsAffected() == 0 {
		return ErrGroupNotFound
	}
	return nil
}

// lockGroup locks an active group row, or returns ErrGroupNotFound.
func lockGroup(ctx context.Context, q querier, id string) error {
	var locked string
	err := q.QueryRow(ctx, "SELECT id FROM public.coach_groups WHERE id = $1 AND is_active FOR UPDATE", id).Scan(&locked)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrGroupNotFound
	}
	if err != nil {
		return fmt.Errorf("lock coach group failed: %w", err)
	}
	return nil
}

func disband(ctx context.Context, q querier, id string) ([]string, error) {
	rows, err := q.Query(ctx, "DELETE FROM public.coach_group_members WHERE group_id = $1 RETURNING user_id", id)
	if err != nil {
		return nil, fmt.Errorf("remove group members failed: %w", err)
	}
	var ids []string
	for rows.Next() {
		var uid string
		if err := rows.Scan(&uid); err != nil {
			rows.Close()
			return nil, err
		}
		ids = append(ids, uid)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if _, err := q.Exec(ctx, `UPDATE public.coach_group_invitations SET status = 'cancelled', responded_at = now()
		WHERE group_id = $1 AND status = 'pending'`, id); err != nil {
		return nil, fmt.Errorf("cancel group invitations failed: %w", err)
	}
	if _, err := q.Exec(ctx, "UPDATE public.coach_groups SET is_active = false, updated_at = now() WHERE id = $1", id); err != nil {
		return nil, fmt.Errorf("deactivate coach group failed: %w", err)
	}
	return ids, nil
}

func (r *pgxGroupRepository) DisbandGroup(ctx context.Context, id string) ([]string, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	if err := lockGroup(ctx, tx, id); err != nil {
		return nil, err
	}
	ids, err := disband(ctx, tx, id)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return ids, nil
}

func (r *pgxGroupRepository) GetMember(ctx context.Context, groupID, userID string) (*Member, error) {
	m, err := scanMember(r.pool.QueryRow(ctx, memberSelect+" WHERE m.group_id = $1 AND m.user_id = $2", groupID, userID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotInGroup
		}
		return nil, fmt.Errorf("get group member failed: %w", err)
	}
	return m, nil
}

func (r *pgxGroupRepository) LeaderIDs(ctx context.Context, groupID string) ([]string, error) {
	rows, err := r.pool.Query(ctx, "SELECT user_id FROM public.coach_group_members WHERE group_id = $1 AND role = 'leader'", groupID)
	if err != nil {
		return nil, fmt.Errorf("list group leaders failed: %w", err)
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// leaderCount counts the group's leaders inside a transaction.
func leaderCount(ctx context.Context, q querier, groupID string) (int, error) {
	var n int
	if err := q.QueryRow(ctx, "SELECT count(*) FROM public.coach_group_members WHERE group_id = $1 AND role = 'leader'", groupID).Scan(&n); err != nil {
		return 0, fmt.Errorf("count group leaders failed: %w", err)
	}
	return n, nil
}

func (r *pgxGroupRepository) UpdateMember(ctx context.Context, groupID, userID string, u MemberUpdate) (*Member, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	if err := lockGroup(ctx, tx, groupID); err != nil {
		return nil, err
	}

	var role string
	err = tx.QueryRow(ctx, "SELECT role FROM public.coach_group_members WHERE group_id = $1 AND user_id = $2", groupID, userID).Scan(&role)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotInGroup
	}
	if err != nil {
		return nil, fmt.Errorf("get member role failed: %w", err)
	}
	if u.Role != nil && role == RoleLeader && *u.Role != RoleLeader {
		n, err := leaderCount(ctx, tx, groupID)
		if err != nil {
			return nil, err
		}
		if n <= 1 {
			return nil, ErrLastLeader
		}
	}

	set := squirrel.Update("public.coach_group_members").PlaceholderFormat(squirrel.Dollar)
	if u.Role != nil {
		set = set.Set("role", *u.Role)
	}
	if u.Status != nil {
		set = set.Set("status", *u.Status)
	}
	sql, args, err := set.Where(squirrel.Eq{"group_id": groupID, "user_id": userID}).ToSql()
	if err != nil {
		return nil, fmt.Errorf("build member update failed: %w", err)
	}
	if _, err := tx.Exec(ctx, sql, args...); err != nil {
		return nil, fmt.Errorf("update group member failed: %w", err)
	}
	m, err := scanMember(tx.QueryRow(ctx, memberSelect+" WHERE m.group_id = $1 AND m.user_id = $2", groupID, userID))
	if err != nil {
		return nil, fmt.Errorf("reload group member failed: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return m, nil
}

func (r *pgxGroupRepository) RemoveMember(ctx context.Context, groupID, userID string) (bool, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer tx.Rollback(ctx)
	if err := lockGroup(ctx, tx, groupID); err != nil {
		return false, err
	}

	var role string
	err = tx.QueryRow(ctx, "SELECT role FROM public.coach_group_members WHERE group_id = $1 AND user_id = $2", groupID, userID).Scan(&role)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, ErrNotInGroup
	}
	if err != nil {
		return false, fmt.Errorf("get member role failed: %w", err)
	}

	var total int
	if err := tx.QueryRow(ctx, "SELECT count(*) FROM public.coach_group_members WHERE group_id = $1", groupID).Scan(&total); err != nil {
		return false, fmt.Errorf("count group members failed: %w", err)
	}
	if total == 1 {
		// The last member leaving dissolves the group.
		if _, err := disband(ctx, tx, groupID); err != nil {
			return false, err
		}
		return true, tx.Commit(ctx)
	}
	if role == RoleLeader {
		n, err := leaderCount(ctx, tx, groupID)
		if err != nil {
			return false, err
		}
		if n <= 1 {
			return false, ErrLastLeader
		}
	}
	if _, err := tx.Exec(ctx, "DELETE FROM public.coach_group_members WHERE group_id = $1 AND user_id = $2", groupID, userID); err != nil {
		return false, fmt.Errorf("remove group member failed: %w", err)
	}
	return false, tx.Commit(ctx)
}

const invitationSelect = `SELECT i.id, i.group_id, g.name, i.invitee_id, COALESCE(u.display_name, u.username),
	i.invited_by, i.role, i.status, i.created_at, i.responded_at
	FROM public.coach_group_invitations i
	JOIN public.coach_groups g ON g.id = i.group_id
	JOIN public.users u ON u.id = i.invitee_id`

func scanInvitation(row pgx.Row, extra ...any) (*Invitation, error) {
	var i Invitation
	dest := append([]any{&i.ID, &i.GroupID, &i.GroupName, &i.InviteeID, &i.InviteeName,
		&i.InvitedBy, &i.Role, &i.Status, &i.CreatedAt, &i.RespondedAt}, extra...)
	if err := row.Scan(dest...); err != nil {
		return nil, err
	}
	return &i, nil
}

func (r *pgxGroupRepository) GetInvitation(ctx context.Context, id string) (*Invitation, error) {
	i, err := scanInvitation(r.pool.QueryRow(ctx, invitationSelect+" WHERE i.id = $1", id))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrInvitationNotFound
		}
		return nil, fmt.Errorf("get invitation failed: %w", err)
	}
	return i, nil
}

func (r *pgxGroupRepository) CreateInvitation(ctx context.Context, groupID, inviteeID, role, invitedBy string) (*Invitation, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	if err := lockGroup(ctx, tx, groupID); err != nil {
		return nil, err
	}

	var inGroup bool
	if err := tx.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM public.coach_group_members WHERE user_id = $1)", inviteeID).Scan(&inGroup); err != nil {
		return nil, fmt.Errorf("check invitee membership failed: %w", err)
	}
	if inGroup {
		return nil, ErrAlreadyInGroup
	}

	var id string
	err = tx.QueryRow(ctx, `INSERT INTO public.coach_group_invitations (group_id, invitee_id, role, invited_by)
		VALUES ($1, $2, $3, $4) RETURNING id`, groupID, inviteeID, role, invitedBy).Scan(&id)
	if err != nil {
		if isUniqueViolation(err) {
			return nil, ErrInvitationPending
		}
		if isFKViolation(err) {
			return nil, ErrInviteeNotCoach
		}
		return nil, fmt.Errorf("create invitation failed: %w", err)
	}
	inv, err := scanInvitation(tx.QueryRow(ctx, invitationSelect+" WHERE i.id = $1", id))
	if err != nil {
		return nil, fmt.Errorf("reload invitation failed: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return inv, nil
}

func (r *pgxGroupRepository) ListInvitations(ctx context.Context, f InvitationFilter) ([]*Invitation, int, error) {
	psql := squirrel.StatementBuilder.PlaceholderFormat(squirrel.Dollar)
	q := psql.Select("i.id", "i.group_id", "g.name", "i.invitee_id", "COALESCE(u.display_name, u.username)",
		"i.invited_by", "i.role", "i.status", "i.created_at", "i.responded_at", "count(*) OVER() AS total_count").
		From("public.coach_group_invitations i").
		Join("public.coach_groups g ON g.id = i.group_id").
		Join("public.users u ON u.id = i.invitee_id").
		OrderBy("i.created_at DESC", "i.id")
	if f.GroupID != "" {
		q = q.Where(squirrel.Eq{"i.group_id": f.GroupID})
	}
	if f.InviteeID != "" {
		q = q.Where(squirrel.Eq{"i.invitee_id": f.InviteeID})
	}
	if f.Status != "" {
		q = q.Where(squirrel.Eq{"i.status": f.Status})
	}
	return pagination.Collect(ctx, r.pool, q, f.Page, f.PageSize, "invitation", func(rows pgx.Rows, total *int) (*Invitation, error) {
		i, err := scanInvitation(rows, total)
		if err != nil {
			return nil, fmt.Errorf("scan invitation failed: %w", err)
		}
		return i, nil
	})
}

func (r *pgxGroupRepository) CancelInvitation(ctx context.Context, groupID, id string) error {
	ct, err := r.pool.Exec(ctx, `UPDATE public.coach_group_invitations SET status = 'cancelled', responded_at = now()
		WHERE id = $1 AND group_id = $2 AND status = 'pending'`, id, groupID)
	if err != nil {
		return fmt.Errorf("cancel invitation failed: %w", err)
	}
	if ct.RowsAffected() == 0 {
		var exists bool
		if err := r.pool.QueryRow(ctx,
			"SELECT EXISTS (SELECT 1 FROM public.coach_group_invitations WHERE id = $1 AND group_id = $2)", id, groupID).Scan(&exists); err != nil {
			return fmt.Errorf("check invitation failed: %w", err)
		}
		if !exists {
			return ErrInvitationNotFound
		}
		return ErrInvitationClosed
	}
	return nil
}

func (r *pgxGroupRepository) RespondInvitation(ctx context.Context, id, inviteeID string, accept bool) (*Invitation, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	var groupID, role, status, invitee string
	err = tx.QueryRow(ctx, `SELECT group_id, role, status, invitee_id FROM public.coach_group_invitations
		WHERE id = $1 FOR UPDATE`, id).Scan(&groupID, &role, &status, &invitee)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && invitee != inviteeID) {
		return nil, ErrInvitationNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("lock invitation failed: %w", err)
	}
	if status != InvitePending {
		return nil, ErrInvitationClosed
	}

	next := InviteDeclined
	if accept {
		next = InviteAccepted
		if err := lockGroup(ctx, tx, groupID); err != nil {
			return nil, err
		}
		if _, err := tx.Exec(ctx,
			"INSERT INTO public.coach_group_members (group_id, user_id, role) VALUES ($1, $2, $3)", groupID, inviteeID, role); err != nil {
			if isUniqueViolation(err) {
				return nil, ErrAlreadyInGroup
			}
			return nil, fmt.Errorf("add group member failed: %w", err)
		}
	}
	if _, err := tx.Exec(ctx,
		"UPDATE public.coach_group_invitations SET status = $2, responded_at = now() WHERE id = $1", id, next); err != nil {
		return nil, fmt.Errorf("update invitation failed: %w", err)
	}
	inv, err := scanInvitation(tx.QueryRow(ctx, invitationSelect+" WHERE i.id = $1", id))
	if err != nil {
		return nil, fmt.Errorf("reload invitation failed: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return inv, nil
}
