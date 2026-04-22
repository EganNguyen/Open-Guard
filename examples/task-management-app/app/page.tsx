'use client';

import React, { useState, useEffect } from 'react';
import {
  LayoutDashboard,
  CheckCircle2,
  Circle,
  Plus,
  Search,
  Bell,
  Settings,
  LogOut,
  Clock,
  AlertCircle,
  Trash2
} from 'lucide-react';

export default function DashboardPage() {
  const [tasks, setTasks] = useState<any[]>([]);
  const [isAdding, setIsAdding] = useState(false);
  const [newTaskTitle, setNewTaskTitle] = useState('');

  useEffect(() => {
    fetchTasks();
  }, []);

  const fetchTasks = async () => {
    const res = await fetch('/api/tasks');
    if (res.ok) {
      const data = await res.json();
      setTasks(data || []);
    }
  };

  const handleAddTask = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!newTaskTitle.trim()) return;
    
    const res = await fetch('/api/tasks', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ title: newTaskTitle })
    });
    
    if (res.ok) {
      setNewTaskTitle('');
      setIsAdding(false);
      fetchTasks();
    }
  };

  const toggleTaskStatus = async (task: any) => {
    const newStatus = task.status === 'completed' ? 'pending' : 'completed';
    const res = await fetch(`/api/tasks/${task.id}`, {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ status: newStatus })
    });
    if (res.ok) {
      fetchTasks();
    }
  };

  const deleteTask = async (taskId: string) => {
    const res = await fetch(`/api/tasks/${taskId}`, {
      method: 'DELETE'
    });
    if (res.ok) {
      fetchTasks();
    }
  };

  const completedTasks = tasks.filter(t => t.status === 'completed').length;
  const pendingTasks = tasks.filter(t => t.status !== 'completed').length;

  return (
    <div className="min-h-screen bg-[#f8fafc] flex">
      {/* Sidebar */}
      <aside className="w-64 bg-slate-900 text-white flex flex-col">
        <div className="p-6">
          <div className="flex items-center gap-3 mb-8">
            <div className="w-8 h-8 bg-blue-500 rounded-lg flex items-center justify-center font-bold">OG</div>
            <span className="text-xl font-bold tracking-tight">Task Management</span>
          </div>

          <nav className="space-y-1">
            <NavItem icon={<LayoutDashboard size={20} />} label="Dashboard" active />
            <NavItem icon={<CheckCircle2 size={20} />} label="My Tasks" />
            <NavItem icon={<Clock size={20} />} label="Recent" />
            <NavItem icon={<Settings size={20} />} label="Settings" />
          </nav>
        </div>

        <div className="mt-auto p-6 border-t border-slate-800">
          <button className="flex items-center gap-3 text-slate-400 hover:text-white transition-colors">
            <LogOut size={20} />
            <span>Sign Out</span>
          </button>
        </div>
      </aside>

      {/* Main Content */}
      <main className="flex-1 flex flex-col">
        {/* Top Header */}
        <header className="h-16 bg-white border-b border-slate-200 flex items-center justify-between px-8">
          <div className="relative w-96">
            <Search className="absolute left-3 top-1/2 -translate-y-1/2 text-slate-400" size={18} />
            <input
              type="text"
              placeholder="Search tasks..."
              className="w-full pl-10 pr-4 py-2 bg-slate-50 border-none rounded-full text-sm focus:ring-2 focus:ring-blue-500 outline-none"
            />
          </div>

          <div className="flex items-center gap-4">
            <button className="p-2 text-slate-500 hover:bg-slate-100 rounded-full relative">
              <Bell size={20} />
              <span className="absolute top-2 right-2 w-2 h-2 bg-red-500 rounded-full border-2 border-white"></span>
            </button>
            <div className="w-8 h-8 rounded-full bg-gradient-to-tr from-blue-500 to-indigo-600 border-2 border-white shadow-sm cursor-pointer"></div>
          </div>
        </header>

        {/* Dashboard Body */}
        <div className="p-8 max-w-5xl">
          <div className="flex items-center justify-between mb-8">
            <div>
              <h1 className="text-2xl font-bold text-slate-900">Task Management</h1>
              <p className="text-slate-500 text-sm">Welcome back! Here's what's happening today.</p>
            </div>
            <button
              onClick={() => setIsAdding(true)}
              className="bg-blue-600 hover:bg-blue-700 text-white px-4 py-2 rounded-lg font-semibold flex items-center gap-2 shadow-lg shadow-blue-500/20 transition-all active:scale-95"
            >
              <Plus size={18} />
              New Task
            </button>
          </div>

          <div className="grid grid-cols-3 gap-6 mb-8">
            <StatCard label="Total Tasks" value={tasks.length.toString()} color="blue" icon={<LayoutDashboard size={20} />} />
            <StatCard label="Completed" value={completedTasks.toString()} color="green" icon={<CheckCircle2 size={20} />} />
            <StatCard label="Pending" value={pendingTasks.toString()} color="amber" icon={<AlertCircle size={20} />} />
          </div>

          {isAdding && (
            <div className="bg-white p-4 rounded-xl shadow-sm border border-slate-200 mb-6 flex gap-3">
              <form onSubmit={handleAddTask} className="flex-1 flex gap-3">
                <input 
                  type="text" 
                  value={newTaskTitle}
                  onChange={(e) => setNewTaskTitle(e.target.value)}
                  placeholder="What needs to be done?" 
                  className="flex-1 bg-slate-50 border border-slate-200 rounded-lg px-4 py-2 focus:ring-2 focus:ring-blue-500 outline-none"
                  autoFocus
                />
                <button type="submit" className="bg-blue-600 hover:bg-blue-700 text-white px-4 py-2 rounded-lg font-medium">Save</button>
                <button type="button" onClick={() => setIsAdding(false)} className="bg-slate-100 hover:bg-slate-200 text-slate-600 px-4 py-2 rounded-lg font-medium">Cancel</button>
              </form>
            </div>
          )}

          {/* Task List */}
          <div className="bg-white rounded-2xl shadow-sm border border-slate-200 overflow-hidden">
            <div className="p-4 border-b border-slate-100 bg-slate-50/50 flex items-center justify-between">
              <span className="text-sm font-semibold text-slate-600 uppercase tracking-wider">Active Tasks</span>
              <button className="text-blue-600 text-xs font-medium hover:underline">View All</button>
            </div>
            <div className="divide-y divide-slate-100">
              {tasks.map((task) => (
                <div key={task.id} className="p-4 flex items-center justify-between hover:bg-slate-50 transition-colors group">
                  <div className="flex items-center gap-4">
                    <button 
                      onClick={() => toggleTaskStatus(task)}
                      className={`w-6 h-6 rounded-full flex items-center justify-center transition-all ${task.status === 'completed' ? 'bg-green-100 text-green-600' : 'border-2 border-slate-200 text-transparent hover:border-blue-400'
                      }`}>
                      <CheckCircle2 size={16} />
                    </button>
                    <div>
                      <h3 className={`font-medium ${task.status === 'completed' ? 'text-slate-400 line-through' : 'text-slate-800'}`}>
                        {task.title}
                      </h3>
                      <div className="flex items-center gap-3 mt-1">
                        <span className={`text-[10px] px-2 py-0.5 rounded-full font-bold uppercase ${task.priority === 'high' ? 'bg-red-50 text-red-500' :
                            task.priority === 'medium' ? 'bg-amber-50 text-amber-500' : 'bg-slate-50 text-slate-500'
                          }`}>
                          {task.priority || 'NORMAL'}
                        </span>
                        <span className="text-xs text-slate-400 flex items-center gap-1">
                          <Clock size={12} />
                          {task.time || 'just now'}
                        </span>
                      </div>
                    </div>
                  </div>
                  <button onClick={() => deleteTask(task.id)} className="text-slate-300 hover:text-red-500 transition-colors">
                    <Trash2 size={16} />
                  </button>
                </div>
              ))}
            </div>
          </div>
        </div>
      </main>
    </div>
  );
}

function NavItem({ icon, label, active = false }: { icon: React.ReactNode; label: string; active?: boolean }) {
  return (
    <a href="#" className={`flex items-center gap-3 px-4 py-3 rounded-xl transition-all ${active ? 'bg-blue-600 text-white shadow-lg shadow-blue-600/20' : 'text-slate-400 hover:text-white hover:bg-slate-800'
      }`}>
      {icon}
      <span className="font-medium">{label}</span>
    </a>
  );
}

function StatCard({ label, value, color, icon }: { label: string; value: string; color: string; icon: React.ReactNode }) {
  const colors = {
    blue: 'bg-blue-50 text-blue-600',
    green: 'bg-green-50 text-green-600',
    amber: 'bg-amber-50 text-amber-600'
  };

  return (
    <div className="bg-white p-6 rounded-2xl border border-slate-200 shadow-sm flex items-center justify-between">
      <div>
        <p className="text-sm font-medium text-slate-500 mb-1">{label}</p>
        <p className="text-3xl font-bold text-slate-900">{value}</p>
      </div>
      <div className={`p-3 rounded-xl ${colors[color]}`}>
        {icon || <LayoutDashboard size={24} />}
      </div>
    </div>
  );
}
