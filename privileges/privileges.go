package privileges

import (
	"database/sql"
	"fmt"
	"log"
	"time"

	"ongrid-thrift/ongrid2"

	"github.com/jmoiron/sqlx"
)

// User ...
type User struct {
	ID        int           `db:"ID"`
	Login     string        `db:"LOGIN"`
	Password  string        `db:"PASSWORD"`
	GroupID   sql.NullInt64 `db:"GROUP_ID"`
	RoleID    sql.NullInt64 `db:"ROLE_ID"`
	CreatedAt time.Time     `db:"CREATED_AT"`
	Active    int           `db:"ACTIVE"`
	Group     *Group
	Role      *Role
}

// Group ...
type Group struct {
	ID          int            `db:"ID"`
	Name        string         `db:"NAME"`
	Description sql.NullString `db:"DESCRIPTION"`
	Roles       []*Role
}

// Role ...
type Role struct {
	ID          int            `db:"ID"`
	Name        string         `db:"NAME"`
	Description sql.NullString `db:"DESCRIPTION"`
	Permissions []*Permission
}

// Permission ...
type Permission struct {
	ID             int       `db:"ID"`
	Name           string    `db:"NAME"`
	PermissionType int       `db:"PERMISSION_TYPE"`
	ObjectID       int       `db:"OBJECTID"`
	CreatedAt      time.Time `db:"CREATED_AT"`
}

type groupRole struct {
	GroupID int `db:"GROUP_ID"`
	RoleID  int `db:"ROLE_ID"`
}

type rolePermission struct {
	RoleID       int       `db:"ROLE_ID"`
	PermissionID int       `db:"PERMISSION_ID"`
	CreatedAt    time.Time `db:"CREATED_AT"`
}

// ACLService ...
type ACLService struct {
	db          *sqlx.DB
	users       map[int]*User
	groups      map[int]*Group
	roles       map[int]*Role
	permissions map[int]*Permission
}

// Load ...
func (s *ACLService) Load(db *sqlx.DB) error {
	s.db = db
	s.users = make(map[int]*User)
	s.groups = make(map[int]*Group)
	s.roles = make(map[int]*Role)
	s.permissions = make(map[int]*Permission)
	s.loadUsers()
	s.loadGroups()
	s.loadRoles()
	s.loadPermissions()

	// Assign group to users
	for _, user := range s.users {
		user.Group = s.groups[int(user.GroupID.Int64)]
		user.Role = s.roles[int(user.RoleID.Int64)]
		fmt.Printf("User: %v\n", user)
	}

	// Assign roles to groups
	groupRoleAssign := []groupRole{}
	err := db.Select(&groupRoleAssign, "select * from og$group_role")
	if err != nil {
		log.Printf("ACLService.Init: %v\n", err)
		return err
	}
	for _, groupRole := range groupRoleAssign {
		groupID := int(groupRole.GroupID)
		roleID := int(groupRole.RoleID)
		s.groups[groupID].assignToRole(s.roles[roleID])
	}

	// Assign permissions to roles
	rolePermissionAssign := []rolePermission{}
	err = db.Select(&rolePermissionAssign, "select * from og$role_permission")
	if err != nil {
		log.Printf("ACLService.Init: %v\n", err)
		return err
	}
	for _, rolePermission := range rolePermissionAssign {
		roleID := int(rolePermission.RoleID)
		permissionID := int(rolePermission.PermissionID)
		s.roles[roleID].assignToPermission(s.permissions[permissionID])
	}

	return nil
}

func (s *ACLService) loadUsers() error {
	rows, err := s.db.Queryx("select * from og$users")
	if err != nil {
		log.Printf("ACLService.loadUsers error: %v", err)
		return err
	}

	for rows.Next() {
		var user User
		err := rows.StructScan(&user)
		if err != nil {
			log.Printf("ACLService.loadUsers, StructScan error: %v", err)
			return err
		}
		fmt.Printf("User: %v\n", user)
		s.users[user.ID] = &user
	}

	return nil
}

func (s *ACLService) loadGroups() error {
	rows, err := s.db.Queryx("select * from og$groups")
	if err != nil {
		log.Printf("ACLService.loadGroups error: %v", err)
		return err
	}

	for rows.Next() {
		var group Group
		err := rows.StructScan(&group)
		if err != nil {
			log.Printf("ACLService.loadUsers, StructScan error: %v", err)
			return err
		}
		fmt.Printf("Group: %v\n", group)
		s.groups[group.ID] = &group
	}

	return nil
}

func (s *ACLService) loadRoles() error {
	rows, err := s.db.Queryx("select * from og$roles")
	if err != nil {
		log.Printf("ACLService.loadRoles error: %v", err)
		return err
	}

	for rows.Next() {
		var role Role
		err := rows.StructScan(&role)
		if err != nil {
			log.Printf("ACLService.loadUsers, StructScan error: %v", err)
			return err
		}
		fmt.Printf("Role: %v\n", role)
		s.roles[role.ID] = &role
	}

	return nil
}

func (s *ACLService) loadPermissions() error {
	rows, err := s.db.Queryx("select * from og$permissions")
	if err != nil {
		log.Printf("ACLService.loadRoles error: %v", err)
		return err
	}

	for rows.Next() {
		var permission Permission
		err := rows.StructScan(&permission)
		if err != nil {
			log.Printf("ACLService.loadPermissions, StructScan error: %v", err)
			return err
		}
		fmt.Printf("Permission: %v\n", permission)
		s.permissions[permission.ID] = &permission
	}

	return nil
}

// GetACL ...
func (s *ACLService) GetACL(userID int) (privileges []*ongrid2.Privilege, err error) {
	permissions := make(map[int]*Permission)

	_, ok := s.users[userID]
	if !ok {
		return nil, fmt.Errorf("User not found, userID: %d", userID)
	}

	for _, role := range s.users[userID].getAllRoles() {
		for _, permission := range role.getPermissions() {
			permissions[permission.ID] = permission
		}
	}

	for _, permission := range permissions {
		var privilege ongrid2.Privilege

		privilege.Resource = int64(permission.ObjectID)
		privilege.Permission = permission.Name
		privilege.Access = (permission.PermissionType == 1)

		privileges = append(privileges, &privilege)
	}

	return
}

func (u *User) getAllRoles() []*Role {
	roles := u.Group.getAllRoles()
	if u.Role != nil {
		roles = append(roles, u.Role)
	}
	return roles
}

func (u *User) getGroup() *Group {
	return u.Group
}

func (g *Group) assignToRole(role *Role) {
	g.Roles = append(g.Roles, role)
}

func (g *Group) getAllRoles() []*Role {
	return g.Roles
}

func (r *Role) getPermissions() []*Permission {
	return r.Permissions
}

func (r *Role) getName() string {
	return r.Name
}

func (r *Role) assignToPermission(permission *Permission) {
	r.Permissions = append(r.Permissions, permission)
}
