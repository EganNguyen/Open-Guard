import React from 'react';
import { Link, useLocation } from 'react-router-dom';
import {
  LayoutDashboard,
  Activity,
  ShieldAlert,
  Network,
  Settings,
  Bell,
} from 'lucide-react';
import clsx from 'clsx';

const navigation = [
  { name: 'Overview', href: '/', icon: LayoutDashboard },
  { name: 'Requests', href: '/requests', icon: Activity },
  { name: 'Threats', href: '/threats', icon: ShieldAlert },
  { name: 'IP Management', href: '/ips', icon: Network },
  { name: 'Configuration', href: '/config', icon: Settings },
  { name: 'Alerts', href: '/alerts', icon: Bell },
];

interface SidebarProps {
  collapsed?: boolean;
}

export function Sidebar({ collapsed = false }: SidebarProps): JSX.Element {
  const location = useLocation();

  return (
    <nav className={clsx(
      'flex flex-col bg-gray-900 text-white',
      collapsed ? 'w-16' : 'w-64'
    )}>
      <div className="p-4 border-b border-gray-800">
        <h1 className={clsx(
          'font-bold text-lg',
          collapsed && 'text-center'
        )}>
          {collapsed ? 'OG' : 'OpenGuard'}
        </h1>
      </div>
      <ul className="flex-1 py-4">
        {navigation.map((item) => {
          const isActive = location.pathname === item.href;
          return (
            <li key={item.name}>
              <Link
                to={item.href}
                className={clsx(
                  'flex items-center gap-3 px-4 py-3 text-sm font-medium transition-colors',
                  isActive
                    ? 'bg-gray-800 text-white'
                    : 'text-gray-400 hover:bg-gray-800 hover:text-white',
                  collapsed && 'justify-center px-2'
                )}
              >
                <item.icon className="w-5 h-5 flex-shrink-0" />
                {!collapsed && <span>{item.name}</span>}
              </Link>
            </li>
          );
        })}
      </ul>
    </nav>
  );
}